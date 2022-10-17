package postgres

import (
	"fmt"

	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type GormConfig struct {
	Host     string
	Port     string
	Database string
	Username string
	Password string
}

func NewGorm(config GormConfig) (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s",
		config.Host, config.Username, config.Password, config.Database, config.Port)
	db, err := gorm.Open(gormpostgres.Open(dsn))
	if err != nil {
		return nil, err
	}
	return db, nil
}
