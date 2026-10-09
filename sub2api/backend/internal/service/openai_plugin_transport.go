package service

import (
	"net/http"
	"time"
)

func (s *OpenAIGatewayService) SetPluginManager(manager *PluginManager) {
	s.pluginManager = manager
}

// doOpenAIUpstream 只在 OpenAI OAuth 能力绑定已启用时把真实请求交给插件。
// 插件返回标准 http.Response，响应解析、错误映射、SSE 和计费仍由现有核心链处理。
func (s *OpenAIGatewayService) doOpenAIUpstream(request *http.Request, proxyURL string, account *Account) (*http.Response, error) {
	var budget *openAICompatFirstOutputBudget
	if limit, ok := request.Context().Value(openAICompatFirstOutputBudgetKey{}).(time.Duration); ok && limit > 0 {
		guarded, activeBudget := newOpenAICompatFirstOutputBudget(request.Context(), limit)
		budget = activeBudget
		request = request.WithContext(guarded)
	}
	finish := func(response *http.Response, err error) (*http.Response, error) {
		if budget == nil {
			return response, err
		}
		if budget.state.Load() == 2 {
			if response != nil && response.Body != nil {
				_ = response.Body.Close()
			}
			budget.close()
			return nil, errOpenAICompatFirstOutputTimeout
		}
		if err != nil || response == nil || response.Body == nil {
			budget.close()
			return response, err
		}
		response.Body = &openAICompatBudgetBody{ReadCloser: response.Body, budget: budget}
		return response, nil
	}
	if s.pluginManager != nil {
		response, handled, err := s.pluginManager.RoundTripOpenAIOAuth(request.Context(), request, proxyURL, account)
		if handled {
			return finish(response, err)
		}
	}
	return finish(s.httpUpstream.Do(request, proxyURL, account.ID, account.Concurrency))
}

// doOpenAIAccountTestUpstream 让 OpenAI OAuth 账号测试与真实转发使用同一插件路径。
// API Key 和未命中插件的账号保持各自原有的 HTTPUpstream 行为。
func (s *AccountTestService) doOpenAIAccountTestUpstream(
	request *http.Request,
	proxyURL string,
	account *Account,
	useTLSFallback bool,
) (*http.Response, error) {
	if s.pluginManager != nil {
		response, handled, err := s.pluginManager.RoundTripOpenAIOAuth(request.Context(), request, proxyURL, account)
		if handled {
			return response, err
		}
	}
	if useTLSFallback {
		return s.httpUpstream.DoWithTLS(
			request,
			proxyURL,
			account.ID,
			account.Concurrency,
			s.tlsFPProfileService.ResolveTLSProfile(account),
		)
	}
	return s.httpUpstream.Do(request, proxyURL, account.ID, account.Concurrency)
}
