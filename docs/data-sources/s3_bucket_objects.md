---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "rabata_s3_bucket_objects Data Source - rabata"
subcategory: ""
description: |-
  
---

# rabata_s3_bucket_objects (Data Source)





<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `bucket` (String)

### Optional

- `delimiter` (String)
- `encoding_type` (String)
- `fetch_owner` (Boolean)
- `max_keys` (Number)
- `prefix` (String)
- `start_after` (String)

### Read-Only

- `common_prefixes` (List of String)
- `id` (String) The ID of this resource.
- `keys` (List of String)
- `owners` (List of String)
