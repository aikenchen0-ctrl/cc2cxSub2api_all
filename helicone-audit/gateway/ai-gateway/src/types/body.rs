use std::{
    convert::Infallible,
    pin::Pin,
    sync::{Arc, LazyLock, atomic::{AtomicBool, Ordering}},
    task::{Context, Poll},
};

pub use axum_core::body::Body;
use bytes::{BufMut, Bytes, BytesMut};
use futures::{Stream, StreamExt};
use hyper::body::{Body as _, Frame, SizeHint};
use tokio::sync::{
    mpsc::{self, Receiver},
    oneshot, Semaphore, OwnedSemaphorePermit,
};

use crate::error::api::ApiError;

// 响应副本共享 128 MiB 实际字节预算，避免小帧数量触发误丢弃。
static AUDIT_RESPONSE_BUDGET: LazyLock<Arc<Semaphore>> = LazyLock::new(|| Arc::new(Semaphore::new(128 * 1024 * 1024)));

#[derive(Debug)]
pub struct CaptureFrame {
    data: Bytes,
    permit: Option<OwnedSemaphorePermit>,
}

/// Reads a stream of HTTP data frames as `Bytes` from a channel.
#[derive(Debug)]
pub struct BodyReader {
    rx: Receiver<CaptureFrame>,
    held_permit: Option<OwnedSemaphorePermit>,
    tfft_tx: Option<oneshot::Sender<()>>,
    is_end_stream: bool,
    size_hint: SizeHint,
    append_newlines: bool,
    capture_complete: Option<Arc<AtomicBool>>,
}

impl BodyReader {
    #[must_use]
    pub fn new(
        rx: Receiver<CaptureFrame>,
        tfft_tx: oneshot::Sender<()>,
        size_hint: SizeHint,
        append_newlines: bool,
    ) -> Self {
        Self {
            rx,
            held_permit: None,
            tfft_tx: Some(tfft_tx),
            is_end_stream: false,
            size_hint,
            append_newlines,
            capture_complete: None,
        }
    }

    /// `append_newlines` is used to support LLM response logging with Helicone
    /// for streaming responses.
    pub fn wrap_stream(
        stream: impl Stream<Item = Result<Bytes, ApiError>> + Send + 'static,
        append_newlines: bool,
    ) -> (axum_core::body::Body, BodyReader, oneshot::Receiver<()>) {
        // 写盘变慢时施加背压，最多排队八个数据帧，避免审计副本无限增长。
        let (tx, rx) = mpsc::channel(8);
        let (tfft_tx, tfft_rx) = oneshot::channel();
        let s = stream.then(move |b| {
            let tx = tx.clone();
            async move {
            if let Ok(b) = &b {
                let chunk_size = if append_newlines { b.len().max(1) } else { 65536 };
                for start in (0..b.len()).step_by(chunk_size) {
                if let Err(e) = tx.send(CaptureFrame { data: b.slice(start..(start + chunk_size).min(b.len())), permit: None }).await {
                    tracing::error!(error = %e, "BodyReader dropped before stream ended");
                    break;
                }
                }
            }
            b
            }
        });
        let client_response = axum_core::body::Body::from_stream(s);
        let size_hint = client_response.size_hint();
        let response_body_for_logger =
            BodyReader::new(rx, tfft_tx, size_hint, append_newlines);
        (client_response, response_body_for_logger, tfft_rx)
    }

    pub fn audit_complete(&self) -> bool {
        self.capture_complete.as_ref().is_none_or(|flag| flag.load(Ordering::Acquire))
    }

    // 审计仅旁路复制；队列满或消费者故障时撤销采集，业务响应永不等待审计。
    pub fn wrap_stream_best_effort(
        stream: impl Stream<Item = Result<Bytes, ApiError>> + Send + 'static,
    ) -> (axum_core::body::Body, BodyReader, oneshot::Receiver<()>) {
        let (tx, rx) = mpsc::channel(1024);
        let (tfft_tx, tfft_rx) = oneshot::channel();
        let complete = Arc::new(AtomicBool::new(false));
        let marker = complete.clone();
        let s = futures::stream::unfold((stream.boxed(), Some(tx)), move |(mut source, mut sender)| {
            let marker = marker.clone();
            async move {
                match source.next().await {
                    Some(result) => {
                        if let Ok(bytes) = &result {
                            for chunk in bytes.chunks(65536) {
                                // 拷贝小片段，避免小切片持有超大上游缓冲区。
                                let permit = AUDIT_RESPONSE_BUDGET.clone().try_acquire_many_owned(chunk.len() as u32).ok();
                                if permit.is_none() || sender.as_ref().is_some_and(|tx| tx.try_send(CaptureFrame { data: Bytes::copy_from_slice(chunk), permit }).is_err()) {
                                    sender = None;
                                    tracing::warn!(reason = "response_queue_full_or_closed", "audit capture skipped; forwarding continues");
                                    break;
                                }
                                if sender.is_none() { break; }
                            }
                        } else { sender = None; }
                        Some((result, (source, sender)))
                    }
                    None => {
                        if sender.is_some() { marker.store(true, Ordering::Release); }
                        None
                    }
                }
            }
        });
        // HTTP 层可能在结束后再次轮询；固定结束状态，避免重复轮询触发异常。
        let client = Body::from_stream(s.fuse());
        let mut reader = BodyReader::new(rx, tfft_tx, SizeHint::default(), false);
        reader.capture_complete = Some(complete);
        (client, reader, tfft_rx)
    }
}

impl hyper::body::Body for BodyReader {
    type Data = Bytes;
    type Error = Infallible;

    fn poll_frame(
        mut self: Pin<&mut Self>,
        cx: &mut Context<'_>,
    ) -> Poll<Option<Result<Frame<Self::Data>, Self::Error>>> {
        // 上一帧已被日志消费者处理，归还对应预算。
        self.held_permit = None;
        match Pin::new(&mut self.rx).poll_recv(cx) {
            Poll::Ready(Some(frame)) => {
                let bytes = frame.data;
                self.held_permit = frame.permit;
                if let Some(tfft_tx) = self.tfft_tx.take() {
                    if let Err(()) = tfft_tx.send(()) {
                        tracing::error!("Failed to send TFFT signal");
                    }
                }

                if self.append_newlines {
                    let mut new_bytes = BytesMut::new();
                    new_bytes.put("data: ".as_bytes());
                    new_bytes.put(bytes);
                    new_bytes.put("\n\n".as_bytes());
                    Poll::Ready(Some(Ok(Frame::data(new_bytes.freeze()))))
                } else {
                    Poll::Ready(Some(Ok(Frame::data(bytes))))
                }
            }
            Poll::Ready(None) => {
                self.is_end_stream = true;
                Poll::Ready(None)
            }
            Poll::Pending => Poll::Pending,
        }
    }

    fn is_end_stream(&self) -> bool {
        self.is_end_stream
    }

    fn size_hint(&self) -> SizeHint {
        self.size_hint.clone()
    }
}
