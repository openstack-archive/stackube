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
	return utils.V2ServiceURL(c, userPath, id)
}

func rootURL(c *gophercloud.ServiceClient) string {
	return utils.V2ServiceURL(c, userPath)
}

func listRolesURL(c *gophercloud.ServiceClient, tenantID, userID string) string {
	return utils.V2ServiceURL(c, tenantPath, tenantID, userPath, userID, rolePath)
}

func listUsersURL(c *gophercloud.ServiceClient, tenantID string) string {
	return utils.V2ServiceURL(c, tenantPath, tenantID, userPath)
}
