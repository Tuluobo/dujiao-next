package contract

import (
	"context"
	"time"

	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/dujiao-next/internal/shared/money"
)

// GatewayPayloadFiatCurrencySent 标记创建支付时实际提交给网关的法币币种。
// 回调兼容逻辑用它区分具备新版事实快照的支付与升级前创建的在途支付。
const GatewayPayloadFiatCurrencySent = "_dujiao_next_fiat_currency_sent"

// GatewayCreateInput 是统一支付网关创建输入。
type GatewayCreateInput struct {
	PaymentID      uint
	OrderID        uint
	OrderNo        string
	Subject        string
	Amount         money.Amount
	Currency       string
	NotifyURL      string
	ReturnURL      string
	ReturnURLQuery map[string]string
	ClientIP       string
	ChannelType    string
	// BuyerEmail 买家邮箱，用于在网关收银台预填。登录用户取账号邮箱，游客取下单时
	// 留的邮箱；两者都没有（或只有 Telegram 占位邮箱）时为空。收银台预填是锦上添花，
	// 因此这个字段允许为空，adapter 必须把空值当作「不传」而不是错误。
	BuyerEmail string
	Extra      jsonmap.JSON
}

// GatewayCreateResult 是统一支付网关创建结果。
type GatewayCreateResult struct {
	ProviderRef        string
	RedirectURL        string
	QRCodeURL          string
	Payload            jsonmap.JSON
	DisplayChannelType string
	AmountSent         string
	CurrencySent       string
}

// GatewayQueryResult 是主动查询网关支付状态的结果。
type GatewayQueryResult struct {
	ProviderRef string
	Status      string
	Amount      money.Amount
	Currency    string
	PaidAt      *time.Time
	Payload     jsonmap.JSON
}

// GatewayCallbackResult 是同步回调或异步 Webhook 的标准结果。
type GatewayCallbackResult struct {
	OrderNo     string
	ProviderRef string
	Status      string
	Amount      money.Amount
	Currency    string
	PaidAt      *time.Time
	Payload     jsonmap.JSON
}

// GatewaySecurityTestResult 是支付网关安全能力的只读诊断结果。
// 结果只包含可公开的校验事实，不包含请求体、私钥或 API 密钥。
type GatewaySecurityTestResult struct {
	VerificationMode         string `json:"verification_mode"`
	ResponseSerial           string `json:"response_serial"`
	RequestSignatureAccepted bool   `json:"request_signature_accepted"`
	ResponseSignatureValid   bool   `json:"response_signature_valid"`
	EchoMessageMatched       bool   `json:"echo_message_matched"`
}

// GatewayProvider 是所有支付网关适配器必须实现的最小能力。
type GatewayProvider interface {
	Type() string
	ValidateConfig(cfg jsonmap.JSON, channelType string) error
	CreatePayment(ctx context.Context, cfg jsonmap.JSON, input GatewayCreateInput) (*GatewayCreateResult, error)
}

type GatewayCapturer interface {
	GatewayProvider
	QueryPayment(ctx context.Context, cfg jsonmap.JSON, providerRef string) (*GatewayQueryResult, error)
}

type GatewayWebhooker interface {
	GatewayProvider
	ParseWebhook(ctx context.Context, cfg jsonmap.JSON, headers map[string]string, body []byte, now time.Time) (*GatewayCallbackResult, error)
}

// GatewaySecurityTester 是支持非交易安全诊断的网关可选能力。
type GatewaySecurityTester interface {
	GatewayProvider
	TestSecurity(ctx context.Context, cfg jsonmap.JSON) (*GatewaySecurityTestResult, error)
}

type GatewayCallbackVerifier interface {
	GatewayProvider
	VerifyCallback(cfg jsonmap.JSON, form map[string][]string, body []byte) (*GatewayCallbackResult, error)
}

// GatewayRegistry 是应用层使用的只读网关注册表端口。
type GatewayRegistry interface {
	Lookup(providerType, channelType string) (GatewayProvider, bool)
}
