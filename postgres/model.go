package postgres

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type User struct {
	ID   uuid.UUID `gorm:"primaryKey;type:uuid"`
	Name string    `gorm:"type:varchar(255)"`
}

func (u *User) BeforeCreate(_ *gorm.DB) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return nil
}

type Item struct {
	ID    uuid.UUID `gorm:"primaryKey;type:uuid"`
	Title string    `gorm:"type:varchar(255)"`
	Price float64
}

func (i *Item) BeforeCreate(_ *gorm.DB) error {
	if i.ID == uuid.Nil {
		i.ID = uuid.New()
	}
	return nil
}

type Point struct {
	ID        uuid.UUID `gorm:"primaryKey;type:uuid"`
	Addr      string    `gorm:"type:varchar(255)"`
	KitchenID uuid.UUID `gorm:"uniqueIndex:kitchen_uniq_idx;type:uuid"`
	CacheID   uuid.UUID `gorm:"uniqueIndex:cache_uniq_idx;type:uuid"`
}

func (p *Point) BeforeCreate(_ *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	if p.KitchenID == uuid.Nil {
		p.KitchenID = uuid.New()
	}
	if p.CacheID == uuid.Nil {
		p.CacheID = uuid.New()
	}
	return nil
}

type Order struct {
	ID         uuid.UUID `gorm:"primaryKey;type:uuid"`
	CreatedAt  time.Time
	Status     string `gorm:"type:varchar(255)"`
	TotalPrice float64
	PINCode    string    `gorm:"type:varchar(255)"`
	UserID     uuid.UUID `gorm:"type:uuid"`
	User       *User
	PointID    uuid.UUID `gorm:"type:uuid"`
	Point      *Point
	Items      []*OrderItem
	LogItems   []*LogItem
}

type OrderItem struct {
	ID         uuid.UUID `gorm:"primaryKey;type:uuid"`
	Title      string    `gorm:"type:varchar(255)"`
	Price      float64
	ItemID     uuid.UUID
	Item       *Item
	Quantity   float64
	TotalPrice float64
	OrderID    uuid.UUID
	Order      *Order
}

type LogItem struct {
	ID      uuid.UUID `gorm:"primaryKey;type:uuid"`
	Text    string
	OrderID uuid.UUID `gorm:"type:uuid"`
	Order   *Order
}

func (p *LogItem) BeforeCreate(_ *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}

type CookItem struct {
	ID        uuid.UUID `gorm:"primaryKey;type:uuid"` // the same as OrderItem.ID
	CreatedAt time.Time
	Title     string `gorm:"type:varchar(255)"`
	Quantity  float64
	KitchenID uuid.UUID `gorm:"type:uuid"`
	OrderID   uuid.UUID `gorm:"type:uuid"`
}

type CacheOrder struct {
	ID               uuid.UUID `gorm:"primaryKey;type:uuid"` // the same as Order.ID
	CreatedAt        time.Time
	CacheID          uuid.UUID `gorm:"type:uuid"`
	OrderID          uuid.UUID `gorm:"type:uuid"`
	UserName         string    `gorm:"type:varchar(255)"`
	Status           string    `gorm:"type:varchar(255)"`
	ReadinessPercent int
	CheckList        string `gorm:"type:varchar(1023)"`
}
