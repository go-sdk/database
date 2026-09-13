package dbx

import (
	"time"

	"github.com/go-sdk/database/dbx/datatype"
)

// Metadata 元数据
type Metadata struct {
	// Id 主键
	Id string `json:"id" gorm:"primaryKey;comment:主键"`

	// 创建人
	CreatedBy string `json:"created_by" gorm:"type:varchar(36);comment:创建人"`
	// 创建时间
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime:milli;comment:创建时间"`
	// 更新人
	UpdatedBy string `json:"updated_by" gorm:"type:varchar(36);comment:更新人"`
	// 更新时间
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime:milli;comment:更新时间"`
	// 删除时间，0 表示有效记录，非 0 值为 Unix 毫秒时间戳
	DeletedAt datatype.DeletedAt `json:"deleted_at" gorm:"not null;default:0;index;comment:删除时间"`
}
