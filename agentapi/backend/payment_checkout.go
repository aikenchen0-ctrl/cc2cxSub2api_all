package main

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/url"
	"strings"
)

// AgentPaymentMethodLimit mirrors the public checkout limit DTO. It contains
// no provider credentials and is safe to return to the AgentAPI browser.
type AgentPaymentMethodLimit struct {
	Currency       string  `json:"currency,omitempty"`
	DisplayName    string  `json:"display_name,omitempty"`
	DailyLimit     float64 `json:"daily_limit"`
	DailyUsed      float64 `json:"daily_used"`
	DailyRemaining float64 `json:"daily_remaining"`
	SingleMin      float64 `json:"single_min"`
	SingleMax      float64 `json:"single_max"`
	FeeRate        float64 `json:"fee_rate"`
	Available      bool    `json:"available"`
}

type AgentCheckoutPlan struct {
	ID                 int64    `json:"id"`
	GroupID            int64    `json:"group_id"`
	GroupPlatform      string   `json:"group_platform,omitempty"`
	GroupName          string   `json:"group_name,omitempty"`
	RateMultiplier     float64  `json:"rate_multiplier,omitempty"`
	PeakRateEnabled    bool     `json:"peak_rate_enabled"`
	PeakStart          string   `json:"peak_start,omitempty"`
	PeakEnd            string   `json:"peak_end,omitempty"`
	PeakRateMultiplier float64  `json:"peak_rate_multiplier,omitempty"`
	DailyLimitUSD      *float64 `json:"daily_limit_usd,omitempty"`
	WeeklyLimitUSD     *float64 `json:"weekly_limit_usd,omitempty"`
	MonthlyLimitUSD    *float64 `json:"monthly_limit_usd,omitempty"`
	ModelScopes        []string `json:"supported_model_scopes"`
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	Price              float64  `json:"price"`
	OriginalPrice      *float64 `json:"original_price,omitempty"`
	Currency           string   `json:"currency,omitempty"`
	ValidityDays       int      `json:"validity_days"`
	ValidityUnit       string   `json:"validity_unit"`
	Features           []string `json:"features"`
	ProductName        string   `json:"product_name"`
}

type AgentCheckoutInfo struct {
	Methods                       map[string]AgentPaymentMethodLimit `json:"methods"`
	GlobalMin                     float64                            `json:"global_min"`
	GlobalMax                     float64                            `json:"global_max"`
	Plans                         []AgentCheckoutPlan                `json:"plans"`
	BalanceDisabled               bool                               `json:"balance_disabled"`
	BalanceRechargeMultiplier     float64                            `json:"balance_recharge_multiplier"`
	SubscriptionUSDToCNYRate      float64                            `json:"subscription_usd_to_cny_rate"`
	RechargeFeeRate               float64                            `json:"recharge_fee_rate"`
	HelpText                      string                             `json:"help_text"`
	HelpImageURL                  string                             `json:"help_image_url"`
	StripePublishableKey          string                             `json:"stripe_publishable_key"`
	AlipayForceQRCode             bool                               `json:"alipay_force_qrcode"`
	AlipayMobilePrecreateDeepLink bool                               `json:"alipay_mobile_precreate_deep_link"`
}

type AgentPaymentOrderRequest struct {
	Amount            float64 `json:"amount"`
	PaymentType       string  `json:"payment_type"`
	OrderType         string  `json:"order_type"`
	PlanID            int64   `json:"plan_id,omitempty"`
	OpenID            string  `json:"openid,omitempty"`
	WechatResumeToken string  `json:"wechat_resume_token,omitempty"`
	IsMobile          *bool   `json:"is_mobile,omitempty"`
	IsWechatBrowser   *bool   `json:"is_wechat_browser,omitempty"`
}

type AgentWechatOAuthInfo struct {
	AuthorizeURL string `json:"authorize_url,omitempty"`
	AppID        string `json:"appid,omitempty"`
	OpenID       string `json:"openid,omitempty"`
	Scope        string `json:"scope,omitempty"`
	State        string `json:"state,omitempty"`
	RedirectURL  string `json:"redirect_url,omitempty"`
}

type AgentWechatJSAPI struct {
	AppID     string `json:"appId,omitempty"`
	TimeStamp string `json:"timeStamp,omitempty"`
	NonceStr  string `json:"nonceStr,omitempty"`
	Package   string `json:"package,omitempty"`
	SignType  string `json:"signType,omitempty"`
	PaySign   string `json:"paySign,omitempty"`
}

type AgentPaymentCreateResult struct {
	OrderID                       int64                 `json:"order_id"`
	Amount                        float64               `json:"amount"`
	PayAmount                     float64               `json:"pay_amount"`
	FeeRate                       float64               `json:"fee_rate"`
	Status                        string                `json:"status"`
	ResultType                    string                `json:"result_type,omitempty"`
	PaymentType                   string                `json:"payment_type,omitempty"`
	OutTradeNo                    string                `json:"out_trade_no,omitempty"`
	PayURL                        string                `json:"pay_url,omitempty"`
	QRCode                        string                `json:"qr_code,omitempty"`
	ClientSecret                  string                `json:"client_secret,omitempty"`
	IntentID                      string                `json:"intent_id,omitempty"`
	Currency                      string                `json:"currency,omitempty"`
	CountryCode                   string                `json:"country_code,omitempty"`
	PaymentEnv                    string                `json:"payment_env,omitempty"`
	OAuth                         *AgentWechatOAuthInfo `json:"oauth,omitempty"`
	JSAPI                         *AgentWechatJSAPI     `json:"jsapi,omitempty"`
	JSAPIPayload                  *AgentWechatJSAPI     `json:"jsapi_payload,omitempty"`
	ExpiresAt                     string                `json:"expires_at"`
	PaymentMode                   string                `json:"payment_mode,omitempty"`
	ResumeToken                   string                `json:"resume_token,omitempty"`
	AlipayMobilePrecreateDeepLink bool                  `json:"alipay_mobile_precreate_deep_link,omitempty"`
}

func (c *MainClient) PaymentCheckoutInfo(ctx context.Context, mainUserID string) (AgentCheckoutInfo, error) {
	data, err := c.satelliteUserJSON(ctx, mainUserID, "/v1/sub2api/payment/checkout-info")
	if err != nil {
		return AgentCheckoutInfo{}, err
	}
	var result AgentCheckoutInfo
	if err := json.Unmarshal(data, &result); err != nil {
		return AgentCheckoutInfo{}, invalidPaymentResponse()
	}
	if result.Methods == nil {
		result.Methods = map[string]AgentPaymentMethodLimit{}
	}
	if result.Plans == nil {
		result.Plans = []AgentCheckoutPlan{}
	}
	for index := range result.Plans {
		if result.Plans[index].Features == nil {
			result.Plans[index].Features = []string{}
		}
		if result.Plans[index].ModelScopes == nil {
			result.Plans[index].ModelScopes = []string{}
		}
	}
	return result, nil
}

func (c *MainClient) CreatePaymentOrder(ctx context.Context, mainUserID string, request AgentPaymentOrderRequest, returnURL string) (AgentPaymentCreateResult, error) {
	payload := map[string]any{
		"amount":         request.Amount,
		"payment_type":   request.PaymentType,
		"order_type":     request.OrderType,
		"return_url":     returnURL,
		"payment_source": "agentapi",
	}
	if request.PlanID > 0 {
		payload["plan_id"] = request.PlanID
	}
	if request.OpenID != "" {
		payload["openid"] = request.OpenID
	}
	if request.WechatResumeToken != "" {
		payload["wechat_resume_token"] = request.WechatResumeToken
	}
	if request.IsMobile != nil {
		payload["is_mobile"] = *request.IsMobile
	}
	if request.IsWechatBrowser != nil {
		payload["is_wechat_browser"] = *request.IsWechatBrowser
	}
	data, err := c.satelliteUserMutationJSON(ctx, mainUserID, "/v1/sub2api/payment/orders", payload)
	if err != nil {
		return AgentPaymentCreateResult{}, err
	}
	var result AgentPaymentCreateResult
	if err := json.Unmarshal(data, &result); err != nil || (result.OrderID <= 0 && result.ResultType != "oauth_required") {
		return AgentPaymentCreateResult{}, invalidPaymentResponse()
	}
	if result.ResultType == "oauth_required" && (result.OAuth == nil || strings.TrimSpace(result.OAuth.AuthorizeURL) == "") {
		return AgentPaymentCreateResult{}, invalidPaymentResponse()
	}
	if result.OAuth != nil && strings.TrimSpace(result.OAuth.AuthorizeURL) != "" {
		result.OAuth.AuthorizeURL, err = c.absoluteWeChatPaymentOAuthURL(result.OAuth.AuthorizeURL)
		if err != nil {
			return AgentPaymentCreateResult{}, invalidPaymentResponse()
		}
	}
	return result, nil
}

// absoluteWeChatPaymentOAuthURL turns the main site's deliberately relative
// OAuth start path into a browser-safe public URL. Only the one known payment
// OAuth route is accepted; an upstream absolute URL or a different path is
// rejected so a compromised/misconfigured main response cannot redirect an
// AgentAPI user to an arbitrary origin.
func (c *MainClient) absoluteWeChatPaymentOAuthURL(raw string) (string, error) {
	authorize, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || authorize.IsAbs() || authorize.Host != "" || authorize.User != nil || authorize.Path != "/api/v1/auth/oauth/wechat/payment/start" {
		return "", invalidPaymentResponse()
	}

	base, ok := publicMainOrigin(c.cfg.PublicMainURL)
	if !ok {
		// Tests and local all-in-one development may omit LINK. Falling back to
		// the API base is safe only for loopback; production must publish LINK.
		apiBase, parseErr := url.Parse(strings.TrimSpace(c.cfg.MainAPIBaseURL))
		if parseErr != nil || apiBase.Host == "" || apiBase.User != nil || !isLoopbackHost(apiBase.Hostname()) || (apiBase.Scheme != "http" && apiBase.Scheme != "https") {
			return "", invalidPaymentResponse()
		}
		base = &url.URL{Scheme: apiBase.Scheme, Host: apiBase.Host}
	}

	base.Path = authorize.Path
	base.RawPath = ""
	base.RawQuery = authorize.RawQuery
	base.Fragment = ""
	return base.String(), nil
}

func (c *MainClient) VerifyPaymentOrder(ctx context.Context, mainUserID, outTradeNo string) (AgentOrder, error) {
	data, err := c.satelliteUserMutationJSON(ctx, mainUserID, "/v1/sub2api/payment/orders/verify", map[string]string{"out_trade_no": outTradeNo})
	if err != nil {
		return AgentOrder{}, err
	}
	var result AgentOrder
	if err := json.Unmarshal(data, &result); err != nil || result.ID <= 0 || result.OutTradeNo == "" {
		return AgentOrder{}, invalidPaymentResponse()
	}
	return result, nil
}

func invalidPaymentResponse() error {
	return &MainAPIError{Status: http.StatusBadGateway, Code: "SATELLITE_INVALID_RESPONSE", Message: "main-site payment service returned an invalid response"}
}

func (s *Server) handleAgentPayment(w http.ResponseWriter, r *http.Request, requestID string) {
	w.Header().Set("Cache-Control", "no-store")
	session, _, ok := s.requireSession(w, r, requestID)
	if !ok {
		return
	}

	const basePath = "/api/v1/agent/payment"
	switch {
	case r.URL.Path == basePath+"/checkout-info" && r.Method == http.MethodGet:
		checkout, err := s.main.PaymentCheckoutInfo(r.Context(), session.MainUserID)
		if err != nil {
			s.writeMainError(w, requestID, err)
			return
		}
		checkout, err = s.applyAgentPlanPolicies(checkout)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to apply agent plan policy")
			return
		}
		s.writeData(w, http.StatusOK, requestID, checkout)
	case r.URL.Path == basePath+"/orders" && r.Method == http.MethodPost:
		if !sameOrigin(r) {
			s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
			return
		}
		var request AgentPaymentOrderRequest
		if err := decodeJSON(r, &request, 64<<10); err != nil {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", "invalid payment order request")
			return
		}
		if err := validateAgentPaymentOrderRequest(request); err != "" {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_PAYMENT_ORDER", err)
			return
		}
		if request.OrderType == "subscription" {
			if err := s.ensureAgentPlanEnabled(request.PlanID); err != nil {
				if errors.Is(err, errAgentPlanDisabled) {
					s.writeError(w, http.StatusBadRequest, requestID, "PLAN_NOT_AVAILABLE", "subscription plan is not available on this agent")
					return
				}
				s.writeError(w, http.StatusInternalServerError, requestID, "STORE_ERROR", "failed to verify agent plan policy")
				return
			}
		}
		result, err := s.main.CreatePaymentOrder(r.Context(), session.MainUserID, request, s.paymentReturnURL(r))
		if err != nil {
			s.writeMainError(w, requestID, err)
			return
		}
		s.recordAudit("agent_user", session.MainUserID, "payment.order.create", "main_order", result.OutTradeNo, requestID, "success", request.OrderType)
		s.writeData(w, http.StatusCreated, requestID, result)
	case r.URL.Path == basePath+"/orders/verify" && r.Method == http.MethodPost:
		if !sameOrigin(r) {
			s.writeError(w, http.StatusForbidden, requestID, "CSRF_ORIGIN_REJECTED", "cross-origin state-changing requests are not allowed")
			return
		}
		var request struct {
			OutTradeNo string `json:"out_trade_no"`
		}
		if err := decodeJSON(r, &request, 16<<10); err != nil {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", "invalid payment verification request")
			return
		}
		request.OutTradeNo = strings.TrimSpace(request.OutTradeNo)
		if len(request.OutTradeNo) < 8 || len(request.OutTradeNo) > 128 || strings.ContainsAny(request.OutTradeNo, "\r\n/\\") {
			s.writeError(w, http.StatusBadRequest, requestID, "INVALID_ORDER_NUMBER", "invalid order number")
			return
		}
		order, err := s.main.VerifyPaymentOrder(r.Context(), session.MainUserID, request.OutTradeNo)
		if err != nil {
			s.writeMainError(w, requestID, err)
			return
		}
		s.writeData(w, http.StatusOK, requestID, order)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, requestID, "METHOD_NOT_ALLOWED", "unsupported payment operation")
	}
}

func validateAgentPaymentOrderRequest(request AgentPaymentOrderRequest) string {
	request.PaymentType = strings.TrimSpace(request.PaymentType)
	request.OrderType = strings.TrimSpace(request.OrderType)
	if request.PaymentType == "" || len(request.PaymentType) > 64 || strings.ContainsAny(request.PaymentType, "\r\n/\\") {
		return "payment_type is invalid"
	}
	if request.OrderType != "balance" && request.OrderType != "subscription" {
		return "order_type must be balance or subscription"
	}
	if math.IsNaN(request.Amount) || math.IsInf(request.Amount, 0) || request.Amount < 0 || request.Amount > 100000000 {
		return "amount is invalid"
	}
	if request.OrderType == "balance" && request.Amount <= 0 && request.WechatResumeToken == "" {
		return "amount must be greater than zero"
	}
	if request.OrderType == "subscription" && request.PlanID <= 0 && request.WechatResumeToken == "" {
		return "plan_id is required for subscription orders"
	}
	if len(request.OpenID) > 512 || strings.ContainsAny(request.OpenID, "\r\n") {
		return "openid is invalid"
	}
	if len(request.WechatResumeToken) > 8192 || strings.ContainsAny(request.WechatResumeToken, "\r\n") {
		return "wechat_resume_token is invalid"
	}
	return ""
}

func (s *Server) paymentReturnURL(r *http.Request) string {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if parsed, err := url.Parse(origin); err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" && sameOrigin(r) {
		return strings.TrimRight(parsed.Scheme+"://"+parsed.Host, "/") + "/payment/result"
	}
	host := strings.TrimSpace(s.cfg.AgentDomain)
	if host == "" {
		host = strings.TrimSpace(r.Host)
	}
	scheme := "https"
	if r.TLS == nil && isLoopbackHost(strings.Split(host, ":")[0]) {
		scheme = "http"
	}
	return scheme + "://" + host + "/payment/result"
}
