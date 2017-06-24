package users

import "github.com/gophercloud/gophercloud"

const (
	tenantPath = "tenants"
	userPath   = "v2.0/users"
	rolePath   = "roles"
)

func ResourceURL(c *gophercloud.ServiceClient, id string) string {
	return c.ServiceURL(userPath, id)
}

func rootURL(c *gophercloud.ServiceClient) string {
	return c.ServiceURL(userPath)
}

func listRolesURL(c *gophercloud.ServiceClient, tenantID, userID string) string {
	return c.ServiceURL(tenantPath, tenantID, userPath, userID, rolePath)
}

func listUsersURL(c *gophercloud.ServiceClient, tenantID string) string {
	return c.ServiceURL(tenantPath, tenantID, "users")
}
