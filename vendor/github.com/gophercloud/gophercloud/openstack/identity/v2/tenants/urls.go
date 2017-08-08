package tenants

import (
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/utils"
)

func listURL(client *gophercloud.ServiceClient) string {
	return utils.V2ServiceURL(client, "tenants")
}

func getURL(client *gophercloud.ServiceClient, tenantID string) string {
	return utils.V2ServiceURL(client, "tenants", tenantID)
}

func createURL(client *gophercloud.ServiceClient) string {
	return utils.V2ServiceURL(client, "tenants")
}

func deleteURL(client *gophercloud.ServiceClient, tenantID string) string {
	return utils.V2ServiceURL(client, "tenants", tenantID)
}

func updateURL(client *gophercloud.ServiceClient, tenantID string) string {
	return utils.V2ServiceURL(client, "tenants", tenantID)
}
