//go:build integration

package leader

import (
	"os"
	"testing"

	"github.com/eventflow/eventflow/libs/go/testx"
)

// TestMain go container Redis dung chung sau khi moi test chay xong.
func TestMain(m *testing.M) {
	code := m.Run()
	testx.Shutdown()
	os.Exit(code)
}
