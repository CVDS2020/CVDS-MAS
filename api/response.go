package api

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func responseError(ctx *gin.Context, err error, header400 bool) {
	responseErrorMsg(ctx, err.Error(), header400)
}

func responseErrorMsg(ctx *gin.Context, err string, header400 bool) {
	if header400 {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code": http.StatusBadRequest,
			"msg":  err,
		})
	} else {
		ctx.JSON(http.StatusOK, gin.H{
			"code": http.StatusBadRequest,
			"msg":  err,
		})
	}
	ctx.Abort()
}

func responseSuccess(ctx *gin.Context, data any) {
	if data != nil {
		ctx.JSON(http.StatusOK, gin.H{
			"code": http.StatusOK,
			"msg":  "success",
			"data": data,
		})
	} else {
		ctx.JSON(http.StatusOK, gin.H{
			"code": http.StatusOK,
			"msg":  "success",
		})
	}
}
