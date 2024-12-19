package aws

import "github.com/aws/aws-sdk-go/aws"

func pointersMapToStringList(pointers map[string]*string) map[string]interface{} {
	list := make(map[string]interface{}, len(pointers))
	for i, v := range pointers {
		list[i] = *v
	}
	return list
}

func stringMapToPointers(m map[string]interface{}) map[string]*string {
	list := make(map[string]*string, len(m))
	for i, v := range m {
		list[i] = aws.String(v.(string))
	}
	return list
}
