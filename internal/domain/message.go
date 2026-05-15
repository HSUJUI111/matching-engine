package domain

const (
	MsgTypeNew    = "NEW"
	MsgTypeCancel = "CANCEL"
)

type KafkaMessage struct {
	Type    string `json:"type"`
	Order   *Order `json:"order,omitempty"`    //New时用
	OrderID int64  `json:"order_id,omitempty"` //Cancel时用
	Symbol  string `json:"symbol,omitempty"`   //Cancel时用于路由分区

}
