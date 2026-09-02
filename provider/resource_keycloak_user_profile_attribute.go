package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/keycloak/terraform-provider-keycloak/keycloak"
)

// resourceKeycloakUserProfileAttribute manages a single attribute of the realm's user profile.
//
// Keycloak only exposes a single GET/PUT endpoint for the whole user profile (see
// keycloak.GetRealmUserProfile / keycloak.UpdateRealmUserProfile), there is no per-attribute API.
// This resource therefore fetches the whole profile, mutates only the attribute it owns (identified
// by its "name"), and writes the whole profile back. This makes it possible to manage some
// attributes with Terraform (using this resource, or the "attribute" blocks of
// keycloak_realm_user_profile), while leaving other attributes untouched, so they can keep being
// created/edited by an admin through the UI or the admin REST API ("unmanaged" attributes).
//
// Ordering of managed and unmanaged attributes: Keycloak preserves the order of the "attributes"
// array as returned by the API. New attributes created by this resource are appended to the end of
// that array, so any pre-existing (unmanaged) attributes keep their relative order. On update, the
// attribute is replaced in place, so its position in the array - and therefore relative to
// unmanaged attributes - never changes. Concurrent create/update/delete operations against the same
// realm are serialized using keycloakClient.Mutex to avoid clobbering concurrent changes made by
// other instances of this resource (or of keycloak_realm_user_profile) within the same Terraform run.
func resourceKeycloakUserProfileAttribute() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceKeycloakUserProfileAttributeCreate,
		ReadContext:   resourceKeycloakUserProfileAttributeRead,
		UpdateContext: resourceKeycloakUserProfileAttributeUpdate,
		DeleteContext: resourceKeycloakUserProfileAttributeDelete,
		Importer: &schema.ResourceImporter{
			StateContext: resourceKeycloakUserProfileAttributeImport,
		},
		Schema: map[string]*schema.Schema{
			"realm_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"display_name": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"default_value": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"multi_valued": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
			"group": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"enabled_when_scope": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"required_for_roles": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"required_for_scopes": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"permissions": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"view": {
							Type:     schema.TypeSet,
							Set:      schema.HashString,
							Required: true,
							Elem:     &schema.Schema{Type: schema.TypeString},
						},
						"edit": {
							Type:     schema.TypeSet,
							Set:      schema.HashString,
							Required: true,
							Elem:     &schema.Schema{Type: schema.TypeString},
						},
					},
				},
			},
			"validator": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": {
							Type:     schema.TypeString,
							Required: true,
						},
						"config": {
							Type:     schema.TypeMap,
							Optional: true,
							Elem:     &schema.Schema{Type: schema.TypeString},
						},
					},
				},
			},
			"annotations": {
				Type:     schema.TypeMap,
				Optional: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
		},
	}
}

func resourceKeycloakUserProfileAttributeCreate(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	keycloakClient := meta.(*keycloak.KeycloakClient)

	realmId := data.Get("realm_id").(string)
	name := data.Get("name").(string)

	keycloakClient.Mutex.Lock(fmt.Sprintf("resourceKeycloakUserProfileAttribute:%s", realmId))
	defer keycloakClient.Mutex.Unlock(fmt.Sprintf("resourceKeycloakUserProfileAttribute:%s", realmId))

	realmUserProfile, err := keycloakClient.GetRealmUserProfile(ctx, realmId)
	if err != nil {
		return diag.FromErr(err)
	}

	for _, attr := range realmUserProfile.Attributes {
		if attr.Name == name {
			return diag.Errorf("attribute %s already exists in the user profile of realm %s. Use terraform import if you want to manage it with this resource", name, realmId)
		}
	}

	attribute := getRealmUserProfileAttributeFromData(mapFromResourceDataToAttributeMap(data))

	if ok, _ := keycloakClient.VersionIsLessThan(ctx, keycloak.Version(minKeycloakDefaultValueVersion)); ok {
		attribute.DefaultValue = ""
	}

	realmUserProfile.Attributes = append(realmUserProfile.Attributes, attribute)

	err = keycloakClient.UpdateRealmUserProfile(ctx, realmId, realmUserProfile)
	if err != nil {
		return diag.FromErr(err)
	}

	data.SetId(fmt.Sprintf("%s/%s", realmId, name))

	return resourceKeycloakUserProfileAttributeRead(ctx, data, meta)
}

func resourceKeycloakUserProfileAttributeRead(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	keycloakClient := meta.(*keycloak.KeycloakClient)

	realmId := data.Get("realm_id").(string)
	name := data.Get("name").(string)

	realmUserProfile, err := keycloakClient.GetRealmUserProfile(ctx, realmId)
	if err != nil {
		return handleNotFoundError(ctx, err, data)
	}

	for _, attr := range realmUserProfile.Attributes {
		if attr.Name == name {
			mapFromAttributeDataToResourceData(data, attr)
			data.Set("realm_id", realmId)
			data.SetId(fmt.Sprintf("%s/%s", realmId, name))
			return nil
		}
	}

	// the attribute is no longer present in the user profile (e.g. it was removed outside of Terraform)
	data.SetId("")

	return nil
}

func resourceKeycloakUserProfileAttributeUpdate(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	keycloakClient := meta.(*keycloak.KeycloakClient)

	realmId := data.Get("realm_id").(string)
	name := data.Get("name").(string)

	keycloakClient.Mutex.Lock(fmt.Sprintf("resourceKeycloakUserProfileAttribute:%s", realmId))
	defer keycloakClient.Mutex.Unlock(fmt.Sprintf("resourceKeycloakUserProfileAttribute:%s", realmId))

	realmUserProfile, err := keycloakClient.GetRealmUserProfile(ctx, realmId)
	if err != nil {
		return diag.FromErr(err)
	}

	attribute := getRealmUserProfileAttributeFromData(mapFromResourceDataToAttributeMap(data))

	if ok, _ := keycloakClient.VersionIsLessThan(ctx, keycloak.Version(minKeycloakDefaultValueVersion)); ok {
		attribute.DefaultValue = ""
	}

	found := false
	for i, attr := range realmUserProfile.Attributes {
		if attr.Name == name {
			// replace in place so the attribute keeps its position relative to any unmanaged attributes
			realmUserProfile.Attributes[i] = attribute
			found = true
			break
		}
	}

	if !found {
		return diag.Errorf("attribute %s no longer exists in the user profile of realm %s", name, realmId)
	}

	err = keycloakClient.UpdateRealmUserProfile(ctx, realmId, realmUserProfile)
	if err != nil {
		return diag.FromErr(err)
	}

	return resourceKeycloakUserProfileAttributeRead(ctx, data, meta)
}

func resourceKeycloakUserProfileAttributeDelete(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	keycloakClient := meta.(*keycloak.KeycloakClient)

	realmId := data.Get("realm_id").(string)
	name := data.Get("name").(string)

	keycloakClient.Mutex.Lock(fmt.Sprintf("resourceKeycloakUserProfileAttribute:%s", realmId))
	defer keycloakClient.Mutex.Unlock(fmt.Sprintf("resourceKeycloakUserProfileAttribute:%s", realmId))

	realmUserProfile, err := keycloakClient.GetRealmUserProfile(ctx, realmId)
	if err != nil {
		return diag.FromErr(err)
	}

	attributes := make([]*keycloak.RealmUserProfileAttribute, 0)
	for _, attr := range realmUserProfile.Attributes {
		if attr.Name != name {
			attributes = append(attributes, attr)
		}
	}
	realmUserProfile.Attributes = attributes

	err = keycloakClient.UpdateRealmUserProfile(ctx, realmId, realmUserProfile)
	if err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func resourceKeycloakUserProfileAttributeImport(_ context.Context, d *schema.ResourceData, _ interface{}) ([]*schema.ResourceData, error) {
	parts := strings.Split(d.Id(), "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid import. Supported import format: {{realmId}}/{{attributeName}}")
	}

	d.Set("realm_id", parts[0])
	d.Set("name", parts[1])
	d.SetId(fmt.Sprintf("%s/%s", parts[0], parts[1]))

	return []*schema.ResourceData{d}, nil
}

// mapFromResourceDataToAttributeMap converts the resource's schema.ResourceData into the
// map[string]interface{} shape expected by getRealmUserProfileAttributeFromData, which is shared
// with the "attribute" blocks of keycloak_realm_user_profile.
func mapFromResourceDataToAttributeMap(data *schema.ResourceData) map[string]interface{} {
	return map[string]interface{}{
		"name":                data.Get("name"),
		"display_name":        data.Get("display_name"),
		"default_value":       data.Get("default_value"),
		"multi_valued":        data.Get("multi_valued"),
		"group":               data.Get("group"),
		"enabled_when_scope":  data.Get("enabled_when_scope"),
		"required_for_roles":  data.Get("required_for_roles"),
		"required_for_scopes": data.Get("required_for_scopes"),
		"permissions":         data.Get("permissions"),
		"validator":           data.Get("validator"),
		"annotations":         data.Get("annotations"),
	}
}

// mapFromAttributeDataToResourceData sets the resource's schema.ResourceData from a
// keycloak.RealmUserProfileAttribute, reusing the same conversion used by keycloak_realm_user_profile.
func mapFromAttributeDataToResourceData(data *schema.ResourceData, attr *keycloak.RealmUserProfileAttribute) {
	attributeData := getRealmUserProfileAttributeData(attr)

	data.Set("name", attributeData["name"])
	data.Set("display_name", attributeData["display_name"])
	data.Set("default_value", attributeData["default_value"])
	data.Set("multi_valued", attributeData["multi_valued"])
	data.Set("group", attributeData["group"])

	if v, ok := attributeData["enabled_when_scope"]; ok {
		data.Set("enabled_when_scope", v)
	}

	data.Set("required_for_roles", attributeData["required_for_roles"])
	data.Set("required_for_scopes", attributeData["required_for_scopes"])

	if v, ok := attributeData["permissions"]; ok {
		data.Set("permissions", v)
	}

	if v, ok := attributeData["validator"]; ok {
		data.Set("validator", v)
	}

	if v, ok := attributeData["annotations"]; ok {
		data.Set("annotations", v)
	}
}
