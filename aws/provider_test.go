package aws

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"os"
	"testing"
)

// testAccPreCheck validates the necessary test API keys exist
// in the testing environment
func testAccPreCheck(t *testing.T) {
	if v := os.Getenv("RABATA_ACCESS_KEY"); v == "" {
		t.Fatal("RABATA_ACCESS_KEY must be set for acceptance tests")
	}
	if v := os.Getenv("RABATA_SECRET_KEY"); v == "" {
		t.Fatal("RABATA_SECRET_KEY must be set for acceptance tests")
	}
}

var testAccProviders map[string]*schema.Provider
var testAccProvider *schema.Provider

func init() {
	testAccProvider = Provider()
	testAccProviders = map[string]*schema.Provider{
		"rabata": testAccProvider,
	}
}
