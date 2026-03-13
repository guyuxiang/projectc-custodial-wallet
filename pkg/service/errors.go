package service

import "github.com/guyuxiang/projectc-wallet/pkg/models"

type AppError struct {
	Code    string
	Message string
}

func (e *AppError) Error() string {
	return e.Message
}

func newAppError(code string, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

func wrapSystemError(err error) *AppError {
	if err == nil {
		return nil
	}
	if appErr, ok := err.(*AppError); ok {
		return appErr
	}
	return newAppError(models.CodeSystemBusy, err.Error())
}
