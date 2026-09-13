package models

import (
	"github.com/go-sdk/database/dbx"
	"github.com/go-sdk/database/dbx/datatype"
)

type User struct {
	dbx.Metadata

	// 用户名
	Name string `json:"name" gorm:"type:varchar(32);not null;comment:用户名"`
	// 邮箱
	Email string `json:"email" gorm:"type:varchar(64);not null;comment:邮箱"`
	// 额外信息
	Extra datatype.JSON `json:"extra" gorm:"comment:额外信息"`
}
