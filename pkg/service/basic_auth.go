package service

import (
	"net/http"
)

// GenBasicAuthMiddleware 生成一个基本的HTTP认证中间件
// 接受用户名和密码作为参数
// 返回一个处理HTTP请求的函数
// 目前用于prometheus访问认证
func GenBasicAuthMiddleware(username string, password string) func(http.ResponseWriter, *http.Request, http.HandlerFunc) {
	return func(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
		requestUsername, requestPassword, ok := r.BasicAuth()
		if !ok || requestUsername != username || requestPassword != password {
			rw.Header().Set("WWW-Authenticate", "Basic realm=\"Protected Area\"")
			rw.WriteHeader(http.StatusUnauthorized)
			return
		}
		next(rw, r)
	}
}
