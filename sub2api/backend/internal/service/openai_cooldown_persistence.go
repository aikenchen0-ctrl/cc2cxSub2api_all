package service

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const (
	openAICooldownPersistenceWorkers = 2
	openAICooldownPersistenceBacklog = 64
	// 不可能作为有效模型名发送给上游，只用于缺少模型信息时的账号写入合并。
	openAIAccountCooldownStateKey = "\x00account_429"
)

type openAICooldownPersistenceTask struct {
	repo      AccountRepository
	state     *openAIAccountModelTransientState
	accountID int64
	model     string
	until     time.Time
}

// 任务上下文脱离客户请求；排队等待不消耗数据库操作的五秒预算。
// 所有退出路径都释放领取状态，数据库失败只影响跨进程传播。
func (task openAICooldownPersistenceTask) run(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, openAIAccountStateUpdateTimeout)
	defer cancel()
	until, claimed := task.until, true
	defer func() {
		if claimed {
			task.state.finishCooldownPersistence(task.accountID, task.model, until, time.Now(), false)
		}
		if recovered := recover(); recovered != nil {
			slog.Error("openai_apikey_429_persist_panicked", "account_id", task.accountID, "error", recovered)
		}
	}()
	for claimed {
		err := ctx.Err()
		if err == nil {
			if task.model == openAIAccountCooldownStateKey {
				err = task.repo.SetRateLimited(ctx, task.accountID, until)
			} else {
				err = task.repo.SetModelRateLimit(ctx, task.accountID, task.model, until, "openai_apikey_429")
			}
		}
		if err != nil {
			slog.Warn("openai_apikey_429_model_persist_failed", "account_id", task.accountID, "model", task.model, "error", err)
		}
		until, claimed = task.state.finishCooldownPersistence(task.accountID, task.model, until, time.Now(), err == nil)
	}
}

// 独立有界队列最多启动两个工作协程；提交永不等待数据库或队列空位。
// 关闭后不再接纳任务，已排队任务利用取消上下文释放其领取状态。
type openAICooldownPersistenceQueue struct {
	mu      sync.Mutex
	stopped bool
	jobs    chan openAICooldownPersistenceTask
	cancel  context.CancelFunc
	done    chan struct{}
	workers int
}

func (q *openAICooldownPersistenceQueue) trySubmit(task openAICooldownPersistenceTask) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.stopped {
		return false
	}
	if q.jobs == nil {
		ctx, cancel := context.WithCancel(context.Background())
		q.cancel = cancel
		q.jobs = make(chan openAICooldownPersistenceTask, openAICooldownPersistenceBacklog)
		q.done = make(chan struct{})
		q.workers = openAICooldownPersistenceWorkers
		for range openAICooldownPersistenceWorkers {
			go q.work(ctx)
		}
	}
	select {
	case q.jobs <- task:
		return true
	default:
		return false
	}
}

func (q *openAICooldownPersistenceQueue) work(ctx context.Context) {
	defer func() {
		q.mu.Lock()
		q.workers--
		if q.workers == 0 {
			close(q.done)
		}
		q.mu.Unlock()
	}()
	for task := range q.jobs {
		task.run(ctx)
	}
}

func (q *openAICooldownPersistenceQueue) stop(wait time.Duration) {
	q.mu.Lock()
	if !q.stopped {
		q.stopped = true
		if q.cancel != nil {
			q.cancel()
			close(q.jobs)
			// 即使正在执行的数据库驱动忽略取消，排队项也应立即解除领取。
			for task := range q.jobs {
				task.state.finishCooldownPersistence(task.accountID, task.model, task.until, time.Now(), false)
			}
		}
	}
	done := q.done
	q.mu.Unlock()
	if done == nil {
		return
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		// 不遵守取消上下文的底层实现也不能无限阻塞服务关闭。
		slog.Warn("openai_apikey_429_persist_stop_timeout")
	}
}

func (s *OpenAIGatewayService) enqueueOpenAICooldownPersistence(state *openAIAccountModelTransientState, accountID int64, model string, now time.Time) {
	until, claimed := state.claimCooldownPersistence(accountID, model, now)
	if !claimed {
		return
	}
	task := openAICooldownPersistenceTask{repo: s.rateLimitService.accountRepo, state: state, accountID: accountID, model: model, until: until}
	if s.deferredService == nil || !s.deferredService.cooldownQueue.trySubmit(task) {
		state.finishCooldownPersistence(accountID, model, until, time.Now(), false)
	}
}
