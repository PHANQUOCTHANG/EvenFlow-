// Package httpx gom phan HTTP dung chung cho moi service Go: loi chuan RFC 7807,
// middleware co ban, tat em va health endpoint.
//
// Muc tieu la de moi service tra loi, ghi log va dung theo CUNG MOT cach: frontend
// va gateway khong phai doan dinh dang loi cua tung service.
package httpx

import (
	"encoding/json"
	"net/http"
)

// WriteJSON ghi v dang JSON voi ma trang thai code.
func WriteJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	// Loi ghi o day nghia la client da ngat ket noi; khong co gi de lam them.
	_ = json.NewEncoder(w).Encode(v)
}

// WriteProblem ghi loi theo RFC 7807 (application/problem+json).
//
// detail di thang toi nguoi dung: KHONG dua loi noi bo (cau SQL, stack, ten host)
// vao day. Chi tiet do chi duoc ghi vao log.
func WriteProblem(w http.ResponseWriter, code int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":   "about:blank",
		"title":  http.StatusText(code),
		"status": code,
		"detail": detail,
	})
}
