package constants

// 锁价单（Price Lock）状态与失效原因统一维护，业务代码禁止散落裸字符串。
const (
	LockStatusActive  = "active"
	LockStatusInvalid = "invalid"

	LockInvalidReasonOutOfStock   = "offer_out_of_stock"
	LockInvalidReasonDiscontinued = "offer_discontinued"

	LockOrderNoPrefix = "LP"
	LockOrderTimeForm = "20060102150405"
	LockOrderRandomTo = 10000
)
