package tenants

import (
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/utils"
)

func listURL(client *gophercloud.ServiceClient) string {
	utils.INsureV2Version(client)
	return client.ServiceURL("tenants")
}

func getURL(client *gophercloud.ServiceClient, tenantID string) string {
	utils.INsureV2Version(client)
	return client.ServiceURL("tenants", tenantID)
}

func createURL(client *gophercloud.ServiceClient) string {
	utils.INsureV2Version(client)
	return client.ServiceURL("tenants")
}

func deleteURL(client *gophercloud.ServiceClient, tenantID string) string {
	utils.INsureV2Version(client)
	return client.ServiceURL("tenants", tenantID)
}

func updateURL(client *gophercloud.ServiceClient, tenantID string) string {
	utils.INsureV2Version(client)
	return client.ServiceURL("tenants", tenantID)
}
