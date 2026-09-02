package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/keycloak/terraform-provider-keycloak/keycloak"
)

func TestAccKeycloakUserProfileAttribute_basic(t *testing.T) {
	realmName := acctest.RandomWithPrefix("tf-acc")
	attributeName := acctest.RandomWithPrefix("tf-acc")

	resource.Test(t, resource.TestCase{
		ProtoV5ProviderFactories: testAccProtoV5ProviderFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccCheckKeycloakUserProfileAttributeDestroy(),
		Steps: []resource.TestStep{
			{
				Config: testKeycloakUserProfileAttribute_basic(realmName, attributeName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckKeycloakUserProfileAttributeExists("keycloak_user_profile_attribute.attribute"),
					resource.TestCheckResourceAttr("keycloak_user_profile_attribute.attribute", "name", attributeName),
					resource.TestCheckResourceAttr("keycloak_user_profile_attribute.attribute", "display_name", "Attribute Display Name"),
					resource.TestCheckResourceAttr("keycloak_user_profile_attribute.attribute", "group", "form-ignore"),
				),
			},
		},
	})
}

func TestAccKeycloakUserProfileAttribute_updateInPlace(t *testing.T) {
	realmName := acctest.RandomWithPrefix("tf-acc")
	attributeName := acctest.RandomWithPrefix("tf-acc")

	resource.Test(t, resource.TestCase{
		ProtoV5ProviderFactories: testAccProtoV5ProviderFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccCheckKeycloakUserProfileAttributeDestroy(),
		Steps: []resource.TestStep{
			{
				Config: testKeycloakUserProfileAttribute_basic(realmName, attributeName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckKeycloakUserProfileAttributeExists("keycloak_user_profile_attribute.attribute"),
					resource.TestCheckResourceAttr("keycloak_user_profile_attribute.attribute", "display_name", "Attribute Display Name"),
				),
			},
			{
				Config: testKeycloakUserProfileAttribute_updated(realmName, attributeName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckKeycloakUserProfileAttributeExists("keycloak_user_profile_attribute.attribute"),
					resource.TestCheckResourceAttr("keycloak_user_profile_attribute.attribute", "display_name", "Updated Display Name"),
				),
			},
		},
	})
}

// TestAccKeycloakUserProfileAttribute_preservesUnmanagedAttributes verifies that creating a
// keycloak_user_profile_attribute resource does not clobber an attribute that was added to the
// user profile outside of Terraform (simulating an attribute created by an admin), and that this
// unmanaged attribute keeps its original position (before the new, Terraform-managed, attribute).
func TestAccKeycloakUserProfileAttribute_preservesUnmanagedAttributes(t *testing.T) {
	realmName := acctest.RandomWithPrefix("tf-acc")
	managedAttributeName := acctest.RandomWithPrefix("tf-acc-managed")
	unmanagedAttributeName := acctest.RandomWithPrefix("tf-acc-unmanaged")

	resource.Test(t, resource.TestCase{
		ProtoV5ProviderFactories: testAccProtoV5ProviderFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccCheckKeycloakUserProfileAttributeDestroy(),
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					// nothing to do before the realm exists; the unmanaged attribute is added
					// via the API right after the realm/user profile is created below.
				},
				Config: testKeycloakUserProfileAttribute_realmOnly(realmName),
				Check: resource.ComposeTestCheckFunc(
					testAccAddUnmanagedUserProfileAttribute(realmName, unmanagedAttributeName),
				),
			},
			{
				Config: testKeycloakUserProfileAttribute_withManagedAttribute(realmName, managedAttributeName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckKeycloakUserProfileAttributeExists("keycloak_user_profile_attribute.attribute"),
					testAccCheckUnmanagedAttributeIsPreservedAndOrdered(realmName, unmanagedAttributeName, managedAttributeName),
				),
			},
		},
	})
}

func testAccAddUnmanagedUserProfileAttribute(realmName, attributeName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		realmUserProfile, err := keycloakClient.GetRealmUserProfile(testCtx, realmName)
		if err != nil {
			return fmt.Errorf("error getting realm user profile: %s", err)
		}

		realmUserProfile.Attributes = append(realmUserProfile.Attributes, &keycloak.RealmUserProfileAttribute{Name: attributeName})

		return keycloakClient.UpdateRealmUserProfile(testCtx, realmName, realmUserProfile)
	}
}

func testAccCheckUnmanagedAttributeIsPreservedAndOrdered(realmName, unmanagedAttributeName, managedAttributeName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		realmUserProfile, err := keycloakClient.GetRealmUserProfile(testCtx, realmName)
		if err != nil {
			return fmt.Errorf("error getting realm user profile: %s", err)
		}

		unmanagedIndex := -1
		managedIndex := -1
		for i, attr := range realmUserProfile.Attributes {
			if attr.Name == unmanagedAttributeName {
				unmanagedIndex = i
			}
			if attr.Name == managedAttributeName {
				managedIndex = i
			}
		}

		if unmanagedIndex == -1 {
			return fmt.Errorf("unmanaged attribute %s was removed from the user profile", unmanagedAttributeName)
		}

		if managedIndex == -1 {
			return fmt.Errorf("managed attribute %s was not found in the user profile", managedAttributeName)
		}

		if unmanagedIndex >= managedIndex {
			return fmt.Errorf("expected unmanaged attribute %s (index %d) to appear before managed attribute %s (index %d)", unmanagedAttributeName, unmanagedIndex, managedAttributeName, managedIndex)
		}

		return nil
	}
}

func testAccCheckKeycloakUserProfileAttributeExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		_, err := getUserProfileAttributeFromState(s, resourceName)
		if err != nil {
			return err
		}

		return nil
	}
}

func testAccCheckKeycloakUserProfileAttributeDestroy() resource.TestCheckFunc {
	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "keycloak_user_profile_attribute" {
				continue
			}

			realmId := rs.Primary.Attributes["realm_id"]
			name := rs.Primary.Attributes["name"]

			realmUserProfile, err := keycloakClient.GetRealmUserProfile(testCtx, realmId)
			if err != nil {
				// realm might already be gone
				continue
			}

			for _, attr := range realmUserProfile.Attributes {
				if attr.Name == name {
					return fmt.Errorf("attribute %s still exists in the user profile of realm %s", name, realmId)
				}
			}
		}

		return nil
	}
}

func getUserProfileAttributeFromState(s *terraform.State, resourceName string) (*keycloak.RealmUserProfileAttribute, error) {
	rs, ok := s.RootModule().Resources[resourceName]
	if !ok {
		return nil, fmt.Errorf("resource not found: %s", resourceName)
	}

	realmId := rs.Primary.Attributes["realm_id"]
	name := rs.Primary.Attributes["name"]

	realmUserProfile, err := keycloakClient.GetRealmUserProfile(testCtx, realmId)
	if err != nil {
		return nil, fmt.Errorf("error getting realm user profile: %s", err)
	}

	for _, attr := range realmUserProfile.Attributes {
		if attr.Name == name {
			return attr, nil
		}
	}

	return nil, fmt.Errorf("attribute %s not found in the user profile of realm %s", name, realmId)
}

func testKeycloakUserProfileAttribute_realmOnly(realm string) string {
	return fmt.Sprintf(`
resource "keycloak_realm" "realm" {
	realm = "%s"

	attributes = {
		userProfileEnabled = true
	}
}

resource "keycloak_realm_user_profile" "realm_user_profile" {
	realm_id                   = keycloak_realm.realm.id
	unmanaged_attribute_policy = "ENABLED"

	attribute {
		name = "username"
	}
	attribute {
		name = "email"
	}
}
`, realm)
}

func testKeycloakUserProfileAttribute_withManagedAttribute(realm, attributeName string) string {
	return fmt.Sprintf(`
resource "keycloak_realm" "realm" {
	realm = "%s"

	attributes = {
		userProfileEnabled = true
	}
}

resource "keycloak_realm_user_profile" "realm_user_profile" {
	realm_id                   = keycloak_realm.realm.id
	unmanaged_attribute_policy = "ENABLED"

	attribute {
		name = "username"
	}
	attribute {
		name = "email"
	}
}

resource "keycloak_user_profile_attribute" "attribute" {
	realm_id = keycloak_realm.realm.id
	name     = "%s"

	depends_on = [keycloak_realm_user_profile.realm_user_profile]
}
`, realm, attributeName)
}

func testKeycloakUserProfileAttribute_basic(realm, attributeName string) string {
	return fmt.Sprintf(`
resource "keycloak_realm" "realm" {
	realm = "%s"

	attributes = {
		userProfileEnabled = true
	}
}

resource "keycloak_realm_user_profile" "realm_user_profile" {
	realm_id                   = keycloak_realm.realm.id
	unmanaged_attribute_policy = "ENABLED"

	attribute {
		name = "username"
	}
	attribute {
		name = "email"
	}
}

resource "keycloak_user_profile_attribute" "attribute" {
	realm_id     = keycloak_realm.realm.id
	name         = "%s"
	display_name = "Attribute Display Name"
	group        = "form-ignore"

	depends_on = [keycloak_realm_user_profile.realm_user_profile]
}
`, realm, attributeName)
}

func testKeycloakUserProfileAttribute_updated(realm, attributeName string) string {
	return fmt.Sprintf(`
resource "keycloak_realm" "realm" {
	realm = "%s"

	attributes = {
		userProfileEnabled = true
	}
}

resource "keycloak_realm_user_profile" "realm_user_profile" {
	realm_id                   = keycloak_realm.realm.id
	unmanaged_attribute_policy = "ENABLED"

	attribute {
		name = "username"
	}
	attribute {
		name = "email"
	}
}

resource "keycloak_user_profile_attribute" "attribute" {
	realm_id     = keycloak_realm.realm.id
	name         = "%s"
	display_name = "Updated Display Name"
	group        = "form-ignore"

	depends_on = [keycloak_realm_user_profile.realm_user_profile]
}
`, realm, attributeName)
}
