package main

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/plugin"
	"github.com/rabataio/terraform-provider-rabata/rabata"
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		ProviderFunc: rabata.Provider,
	})
}
