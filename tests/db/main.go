package main

import (
	"context"

	"github.com/go-sdk/core/errx"
	"github.com/go-sdk/core/logx"
	"github.com/go-sdk/core/osx"
	"github.com/go-sdk/core/seq"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/go-sdk/database/dbx"
	"github.com/go-sdk/database/dbx/datatype"
	"github.com/go-sdk/database/dbx/migrate"
	_ "github.com/go-sdk/database/dbx/sqlite"
	"github.com/go-sdk/database/tests/modelg"
	"github.com/go-sdk/database/tests/models"
)

func main() {
	ctx := context.Background()
	db, err := dbx.Open("sqlite", osx.GetEnv("bin/database.db", "DATABASE_DSN"))
	if err != nil {
		osx.Panic(err)
	}
	migrator, err := migrate.New(db, migrate.Migrations{{
		ID: "20260913_120000_01_create_example_tables",
		Up: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&models.User{}, &models.Bill{})
		},
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&models.Bill{}, &models.User{})
		},
	}})
	if err != nil {
		osx.Panic(err)
	}
	if err := migrator.Up(ctx); err != nil {
		osx.Panic(err)
	}

	// Metadata 主键没有数据库默认值，新增记录必须显式填充 seq.NextID
	userId := seq.NextID()
	billId := seq.NextID()

	cleanup(ctx, db)
	create(ctx, db, userId, billId)
	query(ctx, db, userId, billId)
	update(ctx, db, userId, billId)
	remove(ctx, db, userId, billId)
}

// cleanup 物理清空历史示例数据，使示例可以重复运行
func cleanup(ctx context.Context, db *gorm.DB) {
	if err := db.WithContext(ctx).Unscoped().Where("1 = 1").Delete(&models.User{}).Error; err != nil {
		osx.Panic(err)
	}
	if err := db.WithContext(ctx).Unscoped().Where("1 = 1").Delete(&models.Bill{}).Error; err != nil {
		osx.Panic(err)
	}
}

// create 创建用户和账单
func create(ctx context.Context, db *gorm.DB, userId, billId string) {
	user := models.User{
		Id:    userId,
		Name:  "alice",
		Email: "alice@example.com",
		Extra: datatype.JSON(`{"profile":{"language":"zh-CN"}}`),
	}
	if err := gorm.G[models.User](db).Create(ctx, &user); err != nil {
		osx.Panic(err)
	}
	logx.Info().Msgf("create user: %+v", user)

	bill := models.Bill{
		Id:     billId,
		Amount: decimal.NewFromFloat(12.5),
	}
	if err := gorm.G[models.Bill](db).Create(ctx, &bill); err != nil {
		osx.Panic(err)
	}
	logx.Info().Msgf("create bill: %+v", bill)
}

// query 按主键查询单条记录，并列表查询全部有效记录
func query(ctx context.Context, db *gorm.DB, userId, billId string) {
	user, err := gorm.G[models.User](db).Where(modelg.User.Id.Eq(userId)).First(ctx)
	if err != nil {
		osx.Panic(err)
	}
	logx.Info().Msgf("query user: %+v, language: %s", user, user.Extra.Get("profile.language").String())

	bill, err := gorm.G[models.Bill](db).Where(modelg.Bill.Id.Eq(billId)).First(ctx)
	if err != nil {
		osx.Panic(err)
	}
	logx.Info().Msgf("query bill: %+v, amount: %s", bill, bill.Amount.String())

	users, err := gorm.G[models.User](db).Order(modelg.User.CreatedAt.Desc()).Find(ctx)
	if err != nil {
		osx.Panic(err)
	}
	for _, u := range users {
		logx.Info().Msgf("query user list: %+v", u)
	}
}

// update 更新用户邮箱和账单金额，并回读确认
func update(ctx context.Context, db *gorm.DB, userId, billId string) {
	rows, err := gorm.G[models.User](db).Where(modelg.User.Id.Eq(userId)).Update(ctx, "email", "alice@example.org")
	if err != nil {
		osx.Panic(err)
	}
	logx.Info().Msgf("update user email, rows affected: %d", rows)

	rows, err = gorm.G[models.Bill](db).Where(modelg.Bill.Id.Eq(billId)).Set(modelg.Bill.Amount.Set(decimal.NewFromFloat(99.9))).Update(ctx)
	if err != nil {
		osx.Panic(err)
	}
	logx.Info().Msgf("update bill amount, rows affected: %d", rows)

	user, err := gorm.G[models.User](db).Where(modelg.User.Id.Eq(userId)).First(ctx)
	if err != nil {
		osx.Panic(err)
	}
	bill, err := gorm.G[models.Bill](db).Where(modelg.Bill.Id.Eq(billId)).First(ctx)
	if err != nil {
		osx.Panic(err)
	}
	logx.Info().Msgf("after update, user email: %s, bill amount: %s", user.Email, bill.Amount.String())
}

// remove 软删除用户和账单，普通查询不可见，Unscoped 查询仍可读取
func remove(ctx context.Context, db *gorm.DB, userId, billId string) {
	rows, err := gorm.G[models.User](db).Where(modelg.User.Id.Eq(userId)).Delete(ctx)
	if err != nil {
		osx.Panic(err)
	}
	logx.Info().Msgf("delete user, rows affected: %d", rows)

	rows, err = gorm.G[models.Bill](db).Where(modelg.Bill.Id.Eq(billId)).Delete(ctx)
	if err != nil {
		osx.Panic(err)
	}
	logx.Info().Msgf("delete bill, rows affected: %d", rows)

	if _, err := gorm.G[models.User](db).Where(modelg.User.Id.Eq(userId)).First(ctx); !errx.Is(err, gorm.ErrRecordNotFound) {
		osx.Panic(err)
	}
	if _, err := gorm.G[models.Bill](db).Where(modelg.Bill.Id.Eq(billId)).First(ctx); !errx.Is(err, gorm.ErrRecordNotFound) {
		osx.Panic(err)
	}
	logx.Info().Msg("soft deleted rows are hidden from normal queries")

	var deletedUser models.User
	if err := db.WithContext(ctx).Unscoped().First(&deletedUser, "id = ?", userId).Error; err != nil {
		osx.Panic(err)
	}
	var deletedBill models.Bill
	if err := db.WithContext(ctx).Unscoped().First(&deletedBill, "id = ?", billId).Error; err != nil {
		osx.Panic(err)
	}
	logx.Info().Msgf("unscoped user deleted_at: %d, unscoped bill deleted_at: %d", deletedUser.DeletedAt, deletedBill.DeletedAt)
}
