package rabata_endpoints

import "fmt"

func RabataEndpoint(region string) (string, error) {
	endpoints := map[string]string{
		"us-east-1": "us-east-1.rabata.io",
		"eu-west-1": "rcs.rabata.io",
		"stage":     "stage.rabata.io",
	}
	value, ok := endpoints[region]
	if !ok {
		return "", fmt.Errorf("endpoint for region %q not found", region)
	}
	return value, nil
}
