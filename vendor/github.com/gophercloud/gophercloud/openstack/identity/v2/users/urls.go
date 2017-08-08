package users

import (
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/utils"
)

const (
	tenantPath = "tenants"
	userPath   = "users"
	rolePath   = "roles"
)

func ResourceURL(c *gophercloud.ServiceClient, id string) string {
	utils.INsureV2Version(c)
	return c.ServiceURL(userPath, id)
}

func rootURL(c *gophercloud.ServiceClient) string {
	utils.INsureV2Version(c)
	return c.ServiceURL(userPath)
}

func listRolesURL(c *gophercloud.ServiceClient, tenantID, userID string) string {
	utils.INsureV2Version(c)
	return c.ServiceURL(tenantPath, tenantID, userPath, userID, rolePath)
}

func listUsersURL(c *gophercloud.ServiceClient, tenantID string) string {
	utils.INsureV2Version(c)
	return c.ServiceURL(tenantPath, tenantID, "users")
}
