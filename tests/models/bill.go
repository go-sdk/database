package models

import (
	"github.com/shopspring/decimal"

	"github.com/go-sdk/database/dbx"
)

type Bill struct {
	dbx.Metadata

	// 金额
	Amount decimal.Decimal `json:"amount" gorm:"type:decimal(20,4);not null;default:0;comment:金额"`
}
