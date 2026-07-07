package config

import (
	"errors"
	"strconv"
)

var (
	ErrInvalidServerPort                  = errors.New("invalid server port")
	ErrInvalidMySQLPort                   = errors.New("invalid mysql port")
	ErrMySQLDatabaseNotFound              = errors.New("MySQL database name not found")
	ErrMySQLUserNotFound                  = errors.New("MySQL user not found")
	ErrMySQLHostNotFound                  = errors.New("MySQL host not found")
	ErrInvalidHttpServerReadTimeout       = errors.New("invalid server read time out")
	ErrInvalidHttpServerWriteTimeout      = errors.New("invalid server write time out")
	ErrInvalidHttpServerIdleTimeout       = errors.New("invalid server idle time out")
	ErrInvalidHttpServerReadHeaderTimeout = errors.New("invalid server read header time out")
	ErrInvalidHttpServerMaxHeaderBytes    = errors.New("invalid server max header bytes")
	ErrInvalidHttpServerTimeout           = errors.New("invalid http server time out")
	ErrMySQLMaxOpenConnsFailed            = errors.New("MySQL max open conns failed")
	ErrMySQLMaxIdleConnsFailed            = errors.New("MySQL mysql max idle conns failed")
	ErrMySQLInvalidConnMaxIdleTime        = errors.New("invalid mysql conn max idle time")
	ErrMySQLInvalidConnMaxLifetime        = errors.New("invalid mysql conn max life time")
	ErrMySQLInvalidPingTimeout            = errors.New("invalid mysql conn ping time out")
	ErrInvalidJWTExpireHours              = errors.New("invalid jwt expire hours")
)

func (c Config) Validate() error {
	server := c.Server
	mysql := c.MySQL
	http := c.HttpServer.Server
	jwt := c.JWT

	if server.Port <= 0 {
		return ErrInvalidServerPort
	}

	if mysql.Host == "" {
		return ErrMySQLHostNotFound
	}

	mysqlPort, err := strconv.Atoi(mysql.Port)
	if err != nil || mysqlPort <= 0 || mysqlPort > 65535 {
		return ErrInvalidMySQLPort
	}

	if mysql.Database == "" {
		return ErrMySQLDatabaseNotFound
	}

	if mysql.User == "" {
		return ErrMySQLUserNotFound
	}

	if http.ReadTimeOut <= 0 {
		return ErrInvalidHttpServerReadTimeout
	}

	if http.WriteTimeout <= 0 {
		return ErrInvalidHttpServerWriteTimeout
	}

	if http.IdleTimeout <= 0 {
		return ErrInvalidHttpServerIdleTimeout
	}

	if http.ReadHeaderTimeout <= 0 {
		return ErrInvalidHttpServerReadHeaderTimeout
	}

	if http.MaxHeaderBytesKib <= 0 {
		return ErrInvalidHttpServerMaxHeaderBytes
	}

	if http.Timeout <= 0 {
		return ErrInvalidHttpServerTimeout
	}

	if mysql.MaxOpenConns <= 0 {
		return ErrMySQLMaxOpenConnsFailed
	}

	if mysql.MaxIdleConns < 0 || mysql.MaxIdleConns > mysql.MaxOpenConns {
		return ErrMySQLMaxIdleConnsFailed
	}

	if mysql.ConnMaxIdleTime <= 0 {
		return ErrMySQLInvalidConnMaxIdleTime
	}

	if mysql.ConnMaxLifetime <= 0 {
		return ErrMySQLInvalidConnMaxLifetime
	}

	if mysql.PingTimeout <= 0 {
		return ErrMySQLInvalidPingTimeout
	}

	if jwt.ExpireHours <= 0 {
		return ErrInvalidJWTExpireHours
	}

	if err := c.RabbitMQ.Validate(); err != nil {
		return err
	}

	return nil
}
