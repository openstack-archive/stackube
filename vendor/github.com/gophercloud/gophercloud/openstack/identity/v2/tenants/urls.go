package tenants

import "github.com/gophercloud/gophercloud"

func listURL(client *gophercloud.ServiceClient) string {
	return client.ServiceURL("v2.0/tenants")
}

func getURL(client *gophercloud.ServiceClient, tenantID string) string {
	return client.ServiceURL("v2.0/tenants", tenantID)
}

func createURL(client *gophercloud.ServiceClient) string {
	return client.ServiceURL("v2.0/tenants")
}

func deleteURL(client *gophercloud.ServiceClient, tenantID string) string {
	return client.ServiceURL("v2.0/tenants", tenantID)
}

func updateURL(client *gophercloud.ServiceClient, tenantID string) string {
	return client.ServiceURL("v2.0/tenants", tenantID)
}
