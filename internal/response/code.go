package response

const (
	CodeSuccess        = 0
	CodeParameterError = 1001

	CodeCreateProductFailed    = 1103
	CodeProductNotFound        = 1104
	CodeProductOnSaleFailed    = 1105
	CodeProductAlreadyOffSale  = 1106
	CodeProductOffSaleFailed   = 1107
	CodeQueryProductFailed     = 1108
	CodeQueryProductListFailed = 1109
)

const (
	CodeInitInventoryExists      = 2001
	CodeCreateStockLogFailed     = 2002
	CodeInitInventoryFailed      = 2003
	CodeInventoryInvalidQuantity = 2004
	CodeInventoryNotFound        = 2005
	CodeAddInventoryError        = 2006
	CodeQueryInventoryFailed     = 2007
)

const (
	CodeQueryStockLogFailed = 3001
)

const (
	CodeInsufficientStock          = 4001
	CodeCreateOrderFailed          = 4002
	CodeOrderNotFound              = 4003
	CodeOrderPayFailed             = 4004
	CodeOrderFinishFailed          = 4005
	CodeOrderCancelFailed          = 4006
	CodeQueryOrderListFailed       = 4007
	CodeQueryOrderDetailFailed     = 4008
	CodeOrderNotPaid               = 4009
	CodeOrderAlreadyCanceled       = 4010
	CodeOrderAlreadyFinished       = 4011
	CodeOrderAlreadyPaid           = 4012
	CodeOrderParameterError        = 4013
	CodeOrderIdempotencyConflict   = 4014
	CodeOrderBeingCreated          = 4015
	CodeOrderIdempotencyStateError = 4016
)

const (
	CodeInternalServerError    = 5000
	CodeReadinessFailed        = 5001
	CodeRequestTimeout         = 5002
	CodeDatabaseNotInitialized = 5003
)

const (
	CodeUsernameAlreadyExists    = 6001
	CodeRegisterFailed           = 6002
	CodeUserNotFound             = 6003
	CodeUserDisabled             = 6004
	CodeLoginFailed              = 6005
	CodeUserPasswordNoDifference = 6006
	CodeUpdateUserPasswordFailed = 6007
	CodeNicknameInvalid          = 6008
	CodeUpdateNicknameFailed     = 6009
	CodeInvalidUserID            = 6010
)

const (
	CodeTokenGenerateFailed   = 7001
	CodeTokenMissing          = 7002
	CodeTokenInvalidFormat    = 7003
	CodeTokenExpired          = 7004
	CodeTokenInvalid          = 7005
	CodeTokenMalformed        = 7006
	CodeTokenSignatureInvalid = 7007
	CodeTokenUserInvalid      = 7008
)

const (
	CodeUserRoleCheckFailed  = 8001
	CodeUserRoleNotFound     = 8002
	CodeCreateUserRoleFailed = 8003
	CodePermissionDenied     = 8004
)
