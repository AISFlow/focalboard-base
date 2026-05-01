package sqlstore

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	sq "github.com/Masterminds/squirrel"
	"github.com/mattermost/focalboard/server/model"
	"github.com/mattermost/focalboard/server/utils"

	"github.com/mattermost/mattermost/server/public/shared/mlog"
)

func (s *SQLStore) CloseRows(rows *sql.Rows) {
	if err := rows.Close(); err != nil {
		s.logger.Error("error closing MattermostAuthLayer row set", mlog.Err(err))
	}
}

func (s *SQLStore) IsErrNotFound(err error) bool {
	return model.IsErrNotFound(err)
}

func (s *SQLStore) MarshalJSONB(data interface{}) ([]byte, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	if s.isBinaryParam {
		b = append([]byte{0x01}, b...)
	}

	return b, nil
}

func PrepareNewTestDatabase() (dbType string, connectionString string, err error) {
	dbType = strings.TrimSpace(os.Getenv("FOCALBOARD_STORE_TEST_DB_TYPE"))
	if dbType == "" {
		dbType = model.SqliteDBType
	}
	if dbType == "mariadb" {
		dbType = model.MysqlDBType
	}

	var dbName string
	var rootUser string

	// Extract SSL mode from environment with default fallback
	sslmode := strings.TrimSpace(os.Getenv("FOCALBOARD_DB_SSLMODE"))
	if sslmode == "" {
		sslmode = "disable"
	}

	if dbType == model.SqliteDBType {
		file, err := os.CreateTemp("", "fbtest_*.db")
		if err != nil {
			return "", "", err
		}
		connectionString = file.Name() + "?_busy_timeout=5000"
		_ = file.Close()
	} else if port := strings.TrimSpace(os.Getenv("FOCALBOARD_STORE_TEST_DOCKER_PORT")); port != "" {
		// Prefer test containers over static DSN
		var template string
		switch dbType {
		case model.MysqlDBType:
			template = "%s:mostest@tcp(localhost:%s)/%s?charset=utf8mb4,utf8&writeTimeout=30s&tls=%s"
			rootUser = "root"
		case model.PostgresDBType:
			template = "postgres://%s:mostest@localhost:%s/%s?sslmode=%s&connect_timeout=10"
			rootUser = "mmuser"
		default:
			return "", "", newErrInvalidDBType(dbType)
		}

		var tempConn string
		if dbType == model.PostgresDBType {
			tempConn = fmt.Sprintf(template, rootUser, port, "", sslmode)
		} else {
			tempConn = fmt.Sprintf(template, rootUser, port, "", sslmode)
		}

		sqlDB, err := sql.Open(dbType, tempConn)
		if err != nil {
			return "", "", fmt.Errorf("cannot connect to %s database: %w", dbType, err)
		}
		defer sqlDB.Close()

		if err = sqlDB.Ping(); err != nil {
			return "", "", fmt.Errorf("cannot ping %s database: %w", dbType, err)
		}

		dbName = "testdb_" + utils.NewID(utils.IDTypeNone)[:8]
		if _, err := sqlDB.Exec(fmt.Sprintf("CREATE DATABASE %s;", dbName)); err != nil {
			return "", "", fmt.Errorf("cannot create %s database %s: %w", dbType, dbName, err)
		}

		if dbType != model.PostgresDBType {
			if _, err := sqlDB.Exec(fmt.Sprintf("GRANT ALL PRIVILEGES ON %s.* TO mmuser;", dbName)); err != nil {
				return "", "", fmt.Errorf("cannot grant permissions on %s database %s: %w", dbType, dbName, err)
			}
		}

		if dbType == model.PostgresDBType {
			connectionString = fmt.Sprintf(template, "mmuser", port, dbName, sslmode)
		} else {
			connectionString = fmt.Sprintf(template, "mmuser", port, dbName, sslmode)
		}
	} else {
		// Fall back to raw DSN from environment
		connectionString = strings.TrimSpace(os.Getenv("FOCALBOARD_STORE_TEST_CONN_STRING"))
	}

	return dbType, connectionString, nil
}

type ErrInvalidDBType struct {
	dbType string
}

func newErrInvalidDBType(dbType string) error {
	return ErrInvalidDBType{
		dbType: dbType,
	}
}

func (e ErrInvalidDBType) Error() string {
	return "unsupported database type: " + e.dbType
}

// deleteBoardRecord deletes a boards record without deleting any child records in the blocks table.
// FOR UNIT TESTING ONLY.
func (s *SQLStore) deleteBoardRecord(db sq.BaseRunner, boardID string, modifiedBy string) error {
	return s.deleteBoardAndChildren(db, boardID, modifiedBy, true)
}

// deleteBlockRecord deletes a blocks record without deleting any child records in the blocks table.
// FOR UNIT TESTING ONLY.
func (s *SQLStore) deleteBlockRecord(db sq.BaseRunner, blockID, modifiedBy string) error {
	return s.deleteBlockAndChildren(db, blockID, modifiedBy, true)
}

func (s *SQLStore) castInt(val int64, as string) string {
	if s.dbType == model.MysqlDBType {
		return fmt.Sprintf("cast(%d as unsigned) AS %s", val, as)
	}
	return fmt.Sprintf("cast(%d as bigint) AS %s", val, as)
}

func (s *SQLStore) GetSchemaName() (string, error) {
	var query sq.SelectBuilder

	switch s.dbType {
	case model.MysqlDBType:
		query = s.getQueryBuilder(s.db).Select("DATABASE()")
	case model.PostgresDBType:
		query = s.getQueryBuilder(s.db).Select("current_schema()")
	case model.SqliteDBType:
		return "", nil
	default:
		return "", ErrUnsupportedDatabaseType
	}

	scanner := query.QueryRow()

	var result string
	err := scanner.Scan(&result)
	if err != nil && !model.IsErrNotFound(err) {
		return "", err
	}

	return result, nil
}
