package databases

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"time"
)

type sqliteTime struct {
	Time time.Time
}

func (t *sqliteTime) Scan(value any) error {
	switch v := value.(type) {
	case nil:
		t.Time = time.Time{}
		return nil
	case time.Time:
		t.Time = v
		return nil
	case string:
		return t.scanString(v)
	case []byte:
		return t.scanString(string(v))
	default:
		return fmt.Errorf("unsupported sqlite time value %T", value)
	}
}

func (t sqliteTime) Value() (driver.Value, error) {
	if t.Time.IsZero() {
		return nil, nil
	}
	return t.Time.UTC().Format(time.RFC3339Nano), nil
}

func (t *sqliteTime) scanString(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		t.Time = time.Time{}
		return nil
	}
	for _, layout := range sqliteTimeLayouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			t.Time = parsed.UTC()
			return nil
		}
	}
	return fmt.Errorf("parse sqlite time %q", value)
}

var sqliteTimeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999-07:00",
	"2006-01-02 15:04:05-07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05.999999",
	"2006-01-02 15:04:05",
}
