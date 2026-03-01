package snowflake

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanDescribeKeyValue(t *testing.T) {
	t.Parallel()

	t.Run("PropertyColumns", func(t *testing.T) {
		t.Parallel()

		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		cols := sqlmock.NewRows([]string{"property", "property_value", "extra"}).
			AddRow("TYPE", "STANDARD", "desc")
		mock.ExpectQuery("DESCRIBE").WillReturnRows(cols)

		rows, err := db.Query("DESCRIBE WAREHOUSE")
		require.NoError(t, err)

		result, err := scanDescribeKeyValue(rows)
		require.NoError(t, err)
		assert.Equal(t, "STANDARD", result["TYPE"])
	})

	t.Run("NameValueColumns", func(t *testing.T) {
		t.Parallel()

		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		cols := sqlmock.NewRows([]string{"name", "value"}).
			AddRow("HOST_PORTS", "example.com:443")
		mock.ExpectQuery("DESCRIBE").WillReturnRows(cols)

		rows, err := db.Query("DESCRIBE NETWORK RULE")
		require.NoError(t, err)

		result, err := scanDescribeKeyValue(rows)
		require.NoError(t, err)
		assert.Equal(t, "example.com:443", result["HOST_PORTS"])
	})

	t.Run("UnknownColumnsReturnsError", func(t *testing.T) {
		t.Parallel()

		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		cols := sqlmock.NewRows([]string{"foo", "bar"}).
			AddRow("x", "y")
		mock.ExpectQuery("DESCRIBE").WillReturnRows(cols)

		rows, err := db.Query("DESCRIBE SOMETHING")
		require.NoError(t, err)

		_, err = scanDescribeKeyValue(rows)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot determine key/value columns")
	})

	t.Run("MultipleRows", func(t *testing.T) {
		t.Parallel()

		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		cols := sqlmock.NewRows([]string{"property", "property_value"}).
			AddRow("TYPE", "STANDARD").
			AddRow("SIZE", "XSMALL").
			AddRow("STATE", "STARTED")
		mock.ExpectQuery("DESCRIBE").WillReturnRows(cols)

		rows, err := db.Query("DESCRIBE WAREHOUSE")
		require.NoError(t, err)

		result, err := scanDescribeKeyValue(rows)
		require.NoError(t, err)
		assert.Equal(t, "STANDARD", result["TYPE"])
		assert.Equal(t, "XSMALL", result["SIZE"])
		assert.Equal(t, "STARTED", result["STATE"])
	})
}
