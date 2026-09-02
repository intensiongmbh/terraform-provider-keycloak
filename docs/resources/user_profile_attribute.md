---
page_title: "keycloak_user_profile_attribute Resource"
---

# keycloak_user_profile_attribute Resource

Allows for managing a single attribute of a realm's user profile.

Keycloak only exposes a single "get/put the whole user profile" API - there is no dedicated API to
manage a single attribute. This resource works around that limitation by fetching the whole user
profile, updating only the attribute it owns (matched by `name`), and writing the whole profile
back, leaving every other attribute (and group) untouched.

This makes it possible to mix:

- **Managed attributes**: attributes defined either with this resource, or with the `attribute`
  blocks of the [`keycloak_realm_user_profile`](realm_user_profile.md) resource.
- **Unmanaged attributes**: attributes created and maintained outside of this Terraform
  configuration, e.g. by an admin through the Keycloak Admin Console or the Admin REST API.

~> Do not mix `keycloak_user_profile_attribute` resources and `attribute` blocks inside
`keycloak_realm_user_profile` for the *same* realm. The `attribute` blocks of
`keycloak_realm_user_profile` always reconcile the *entire* attribute list to exactly match the
list declared in HCL, which would remove any attribute managed only through
`keycloak_user_profile_attribute` (or created by an admin). Pick one approach per realm: either
manage every attribute through `keycloak_realm_user_profile`'s `attribute` blocks, or manage
individual attributes through `keycloak_user_profile_attribute` and leave `keycloak_realm_user_profile`
without any `attribute` block.

## Ordering of managed and unmanaged attributes

Keycloak preserves the order of the `attributes` array of the user profile. When a
`keycloak_user_profile_attribute` is created, it is appended to the end of that array, so any
attribute created earlier - whether managed by Terraform or created by an admin - keeps its
relative position. When the attribute is updated, it is replaced in place (same index), so its
position never changes across updates. Deleting the resource removes only that attribute from the
array, again without touching the position of any other attribute.

In practice this means the resulting order is: whatever attributes already existed (in their
existing order), followed by the Terraform-managed attributes in the order they were created.

## Example Usage

```hcl
resource "keycloak_realm" "realm" {
  realm = "my-realm"
}

resource "keycloak_realm_user_profile" "userprofile" {
  realm_id                   = keycloak_realm.realm.id
  unmanaged_attribute_policy = "ENABLED"
}

resource "keycloak_user_profile_attribute" "example" {
  realm_id = keycloak_realm.realm.id

  name         = "example"
  display_name = "$${profile.attribute.example}"
  group        = "form-ignore"

  multi_valued        = false
  enabled_when_scope   = ["offline_access"]
  required_for_roles  = ["user"]
  required_for_scopes = ["offline_access"]

  permissions {
    view = ["admin", "user"]
    edit = ["admin", "user"]
  }

  validator {
    name = "person-name-prohibited-characters"
  }

  validator {
    name = "pattern"
    config = {
      pattern       = "^[a-z0-9 ]+$"
      error-message = "Nope"
    }
  }

  annotations = {
    foo = "bar"
  }
}
```

## Argument Reference

- `realm_id` - (Required) The ID of the realm the attribute applies to. Changing this forces a new resource.
- `name` - (Required) The name of the attribute. Changing this forces a new resource.
- `display_name` - (Optional) The display name of the attribute.
- `default_value` - (Optional) The default value of the attribute. Only applied with Keycloak 26.4.0 or later.
- `multi_valued` - (Optional) If the attribute supports multiple values. Defaults to `false`.
- `group` - (Optional) The group that the attribute belongs to.
- `enabled_when_scope` - (Optional) A list of scopes. The attribute will only be enabled when these scopes are requested by clients.
- `required_for_roles` - (Optional) A list of roles for which the attribute will be required.
- `required_for_scopes` - (Optional) A list of scopes for which the attribute will be required.
- `permissions` - (Optional) The [permissions](#permissions-arguments) configuration information.
- `validator` - (Optional) A list of [validators](#validator-arguments) for the attribute.
- `annotations` - (Optional) A map of annotations for the attribute. Values can be a String or a json object.

### Permissions Arguments

- `edit` - (Optional) A list of profiles that will be able to edit the attribute. One of `admin`, `user`.
- `view` - (Optional) A list of profiles that will be able to view the attribute. One of `admin`, `user`.

### Validator Arguments

- `name` - (Required) The name of the validator.
- `config` - (Optional) A map defining the configuration of the validator. Values can be a String or a json object.

## Import

This resource can be imported using the format `{{realm_id}}/{{attribute_name}}`.

```bash
$ terraform import keycloak_user_profile_attribute.example my-realm/example
```
