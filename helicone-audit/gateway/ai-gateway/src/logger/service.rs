use std::{time::Duration, sync::{Arc, LazyLock}};

use bytes::Bytes;
use chrono::{DateTime, Utc};
use http::{HeaderMap, StatusCode};
use http_body_util::BodyExt;
use indexmap::IndexMap;
use opentelemetry::KeyValue;
use reqwest::Client;
use tokio::{sync::{oneshot, Semaphore, OwnedSemaphorePermit}, time::Instant, io::AsyncWriteExt};
use typed_builder::TypedBuilder;
use url::Url;
use uuid::Uuid;

use crate::{
    app_state::AppState,
    config::deployment_target::DeploymentTarget,
    error::{init::InitError, logger::LoggerError},
    metrics::tfft::TFFTFuture,
    store::minio::MinioClient,
    types::{
        body::BodyReader,
        extensions::{AuthContext, MapperContext, PromptContext},
        logger::{
            HeliconeLogMetadata, Log, LogMessage, RequestLog, ResponseLog,
        },
        provider::InferenceProvider,
        router::RouterId,
    },
};

const JAWN_CONNECT_TIMEOUT: Duration = Duration::from_secs(10);
// 名额只限制旁路审计；获取失败时直接跳过采集，不阻塞业务转发。
pub static AUDIT_SLOTS: LazyLock<Arc<Semaphore>> = LazyLock::new(|| Arc::new(Semaphore::new(512)));
// 每个令牌代表 64 KiB，所有请求正文合计最多预留 256 MiB。
pub static AUDIT_BODY_BUDGET: LazyLock<Arc<Semaphore>> = LazyLock::new(|| Arc::new(Semaphore::new(4096)));

struct SpoolFile(std::path::PathBuf, bool);
impl Drop for SpoolFile {
    fn drop(&mut self) {
        if !self.1 {
            let path = self.0.clone();
            // 清理也走异步文件接口，避免故障磁盘的删除操作阻塞业务运行线程。
            tokio::spawn(async move { let _ = tokio::fs::remove_file(path).await; });
        }
    }
}

// 按字节转义 JSON 字符串，UTF-8 多字节序列保持原样，不受帧边界影响。
async fn write_json_bytes(file: &mut tokio::fs::File, bytes: &[u8]) -> Result<(), LoggerError> {
    for chunk in bytes.chunks(65536) {
        let mut escaped = Vec::with_capacity(chunk.len() * 2);
        for &byte in chunk {
            match byte {
                b'"' => escaped.extend_from_slice(b"\\\""),
                b'\\' => escaped.extend_from_slice(b"\\\\"),
                0..=31 => escaped.extend_from_slice(format!("\\u{byte:04x}").as_bytes()),
                _ => escaped.push(byte),
            }
        }
        file.write_all(&escaped).await?;
    }
    Ok(())
}

#[derive(Debug)]
pub struct JawnClient {
    pub request_client: Client,
}

impl JawnClient {
    pub fn new() -> Result<Self, InitError> {
        Ok(Self {
            request_client: Client::builder()
                .tcp_nodelay(true)
                .connect_timeout(JAWN_CONNECT_TIMEOUT)
                .timeout(Duration::from_secs(60))
                .build()
                .map_err(InitError::CreateReqwestClient)?,
        })
    }
}

#[derive(Debug, TypedBuilder)]
pub struct LoggerService {
    #[builder(default)]
    audit_permit: Option<OwnedSemaphorePermit>,
    #[builder(default)]
    memory_permit: Option<OwnedSemaphorePermit>,
    app_state: AppState,
    auth_ctx: AuthContext,
    start_time: DateTime<Utc>,
    start_instant: Instant,
    response_body: BodyReader,
    request_body: Bytes,
    target_url: Url,
    request_headers: HeaderMap,
    response_status: StatusCode,
    provider: InferenceProvider,
    mapper_ctx: MapperContext,
    router_id: Option<RouterId>,
    deployment_target: DeploymentTarget,
    tfft_rx: oneshot::Receiver<()>,
    request_id: Uuid,
    #[builder(default)]
    cache_enabled: Option<bool>,
    #[builder(default)]
    cache_bucket_max_size: Option<u8>,
    #[builder(default)]
    cache_control: Option<String>,
    #[builder(default)]
    cache_reference_id: Option<String>,
    #[builder(default)]
    prompt_ctx: Option<PromptContext>,
}

impl LoggerService {
    #[tracing::instrument(skip_all)]
    #[allow(clippy::cast_precision_loss, clippy::too_many_lines)]
    pub async fn log(mut self) -> Result<(), LoggerError> {
        let _audit_permit = self.audit_permit.take();
        tracing::trace!("logging request");
        let tfft_future = TFFTFuture::new(self.start_instant, self.tfft_rx);
        let req_body_len = self.request_body.len();
        let resp_body_len;
        let tfft_duration;
        let mut queued_spool = None;
        if self.app_state.config().server.forward_client_authorization {
            let durable_root = std::env::var_os("CC2_AUDIT_SPOOL").map(std::path::PathBuf::from);
            if let Some(root) = &durable_root {
                if !tokio::fs::try_exists(root.join("capture-enabled")).await? {
                    return Err(LoggerError::CaptureIncomplete);
                }
                let modified = tokio::fs::metadata(root.join("capture-enabled")).await?.modified()?;
                if modified.elapsed().unwrap_or_default() > Duration::from_secs(30) {
                    return Err(LoggerError::CaptureIncomplete);
                }
            }
            let path = durable_root.as_ref().map_or_else(
                || std::env::temp_dir().join(format!("cc2-audit-{}.json", self.request_id)),
                |root| root.join(format!("{}.body.partial", self.request_id)));
            let mut spool = SpoolFile(path, false);
            let mut file = tokio::fs::OpenOptions::new().write(true).create_new(true).mode(0o600).open(&spool.0).await?;
            file.write_all(b"{\"request\":\"").await?;
            write_json_bytes(&mut file, &self.request_body).await?;
            self.request_body = Bytes::new();
            drop(self.memory_permit.take());
            file.write_all(b"\",\"response\":\"").await?;
            let collect_future = async {
            let mut total = 0usize;
            while let Some(frame) = self.response_body.frame().await {
                if let Ok(bytes) = frame.expect("infallible body").into_data() {
                    total += bytes.len();
                    // 超大响应只撤销本条采集，消费者退出不会阻塞业务流。
                    if total > 32 * 1024 * 1024 { return Err(LoggerError::CaptureIncomplete); }
                    write_json_bytes(&mut file, &bytes).await?;
                }
            }
            Ok::<usize, LoggerError>(total)
            };
            let (total, measured_tfft) = tokio::join!(collect_future, tfft_future);
            if !self.response_body.audit_complete() {
                return Err(LoggerError::CaptureIncomplete);
            }
            file.write_all(b"\"}").await?;
            file.flush().await?;
            if durable_root.is_some() { file.sync_all().await?; }
            drop(file);
            resp_body_len = total?;
            tfft_duration = measured_tfft.unwrap_or_default();
            if let Some(root) = durable_root {
                let final_path = root.join(format!("{}.body", self.request_id));
                tokio::fs::rename(&spool.0, &final_path).await?;
                spool.0 = final_path;
                queued_spool = Some(spool);
            } else {
                MinioClient::sidecar(&self.app_state.0.jawn_http_client)
                    .log_body_file(&self.app_state, &self.auth_ctx, self.request_id, &spool.0).await?;
            }
        } else {
        let collect_future = self.response_body.collect();
        let (response_body, measured_tfft) =
            tokio::join!(collect_future, tfft_future);
        let response_body = response_body
            .inspect_err(|_| tracing::error!("infallible errored"))
            .expect("infallible never errors")
            .to_bytes();
        tfft_duration = measured_tfft.unwrap_or_else(|_| {
            tracing::error!("Failed to get TFFT signal");
            Duration::from_secs(0)
        });
        tracing::trace!(tfft_duration = ?tfft_duration, "tfft_duration");
        resp_body_len = response_body.len();
        let s3_client = if self.app_state.config().deployment_target.is_cloud()
        {
            MinioClient::cloud(&self.app_state.0.minio)
        } else {
            MinioClient::sidecar(&self.app_state.0.jawn_http_client)
        };
        s3_client
            .log_bodies(
                &self.app_state,
                &self.auth_ctx,
                self.request_id,
                self.request_body,
                response_body,
            )
            .await?;
        }

        let model = self
            .mapper_ctx
            .model
            .as_ref()
            .map_or_else(|| "unknown".to_string(), ToString::to_string);
        let attributes = [
            KeyValue::new("provider", self.provider.to_string()),
            KeyValue::new("model", model),
            KeyValue::new("path", self.target_url.path().to_string()),
        ];
        self.app_state
            .0
            .metrics
            .tfft_duration
            .record(tfft_duration.as_millis() as f64, &attributes);

        let helicone_metadata = HeliconeLogMetadata::from_headers(
            &mut self.request_headers,
            self.router_id,
            &self.deployment_target,
            self.prompt_ctx,
        )?;
        let req_path = self.target_url.path().to_string();
        let provider = match self.provider {
            InferenceProvider::Ollama => "CUSTOM".to_string(),
            InferenceProvider::GoogleGemini => "GOOGLE".to_string(),
            provider => provider.to_string().to_uppercase(),
        };

        // Extract helicone-properties-* headers
        let mut properties = IndexMap::new();
        for (name, value) in &self.request_headers {
            if name.as_str().starts_with("helicone-property-") {
                if let Ok(value_str) = value.to_str() {
                    properties.insert(name.to_string(), value_str.to_string());
                }
            }
        }

        let request_log = RequestLog::builder()
            .id(self.request_id)
            .user_id(self.auth_ctx.user_id)
            .properties(properties)
            .target_url(self.target_url)
            .provider(provider)
            .body_size(req_body_len as f64)
            .path(req_path)
            .request_created_at(self.start_time)
            .is_stream(self.mapper_ctx.is_stream)
            .cache_enabled(self.cache_enabled)
            .cache_bucket_max_size(self.cache_bucket_max_size)
            .cache_control(self.cache_control)
            .cache_reference_id(self.cache_reference_id)
            .build();
        let response_log = ResponseLog::builder()
            .id(self.request_id)
            .status(f64::from(self.response_status.as_u16()))
            .body_size(resp_body_len as f64)
            .response_created_at(Utc::now())
            .delay_ms(tfft_duration.as_millis() as f64)
            .build();
        let log = Log::new(request_log, response_log);
        let mut log_message = LogMessage::builder()
            .authorization(self.auth_ctx.api_key.expose().to_string())
            .helicone_meta(helicone_metadata)
            .log(log)
            .build();

        if let Some(mut spool) = queued_spool {
            // 先提交完整正文，再原子提交元数据；上传进程只读取 ready 文件。
            // 凭证由独立上传进程从受限配置读取，持久队列不保存审计密钥。
            log_message.authorization.clear();
            let bytes = serde_json::to_vec(&log_message).map_err(|_| LoggerError::InvalidLogMessage)?;
            let root = spool.0.parent().expect("spool has parent");
            let partial = root.join(format!("{}.meta.partial", self.request_id));
            let mut meta = tokio::fs::OpenOptions::new().write(true).create_new(true).mode(0o600).open(&partial).await?;
            meta.write_all(&bytes).await?;
            meta.sync_all().await?;
            drop(meta);
            tokio::fs::rename(&partial, root.join(format!("{}.ready", self.request_id))).await?;
            spool.1 = true;
            tokio::fs::File::open(root).await?.sync_all().await?;
            return Ok(());
        }

        let helicone_url = self
            .app_state
            .config()
            .helicone
            .base_url
            .join("/v1/log/request")?;

        let _helicone_response = self
            .app_state
            .0
            .jawn_http_client
            .request_client
            .post(helicone_url)
            .json(&log_message)
            .header(
                "authorization",
                format!("Bearer {}", self.auth_ctx.api_key.expose()),
            )
            .send()
            .await
            .map_err(|e| {
                tracing::debug!(error = %e, "failed to send request to helicone");
                LoggerError::FailedToSendRequest(e)
            })?
            .error_for_status()
            .map_err(|e| {
                tracing::error!(error = %e, "failed to log request to helicone");
                LoggerError::ResponseError(e)
            })?;

        tracing::debug!("successfully logged request");
        Ok(())
    }
}
