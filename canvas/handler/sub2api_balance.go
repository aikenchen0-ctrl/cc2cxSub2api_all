package handler

import (
	"net/http"

	"github.com/tigerowo/infinite-canvas/repository"
	"github.com/tigerowo/infinite-canvas/service"
)

type sub2APIUserBalanceResponse struct {
	Balance     float64 `json:"balance"`
	RechargeURL string  `json:"recharge_url"`
}

func Sub2APIUserBalance(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		FailWithStatus(w, http.StatusUnauthorized, "请先登录")
		return
	}
	account, ok, err := repository.GetUserByID(user.ID)
	if err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "读取账户身份失败")
		return
	}
	if !ok || account.Sub2APISubject == "" {
		FailWithStatus(w, http.StatusUnauthorized, "当前会话未关联 Sub2API 账户")
		return
	}
	balance, status, err := service.FetchSub2APIUserBalance(r.Context(), account.Sub2APISubject)
	if err != nil {
		FailWithStatus(w, status, "读取 Sub2API 余额失败")
		return
	}
	OK(w, sub2APIUserBalanceResponse{Balance: balance, RechargeURL: service.Sub2APIPurchaseURL()})
}
