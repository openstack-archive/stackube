/*
Copyright (c) 2017 OpenStack Foundation.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package types

type Network struct {
	Name      string
	Uid       string
	TenantID  string
	SegmentID int32
	Subnets   []*Subnet
	// Status of network
	// Valid value: Initializing, Active, Pending, Failed, Terminating
	Status string
}

// Subnet is a representaion of a subnet
type Subnet struct {
	Name       string
	Uid        string
	Cidr       string
	Gateway    string
	Tenantid   string
	Dnsservers []string
	Routes     []*Route
}

// Route is a representation of an advanced routing rule.
type Route struct {
	Name            string
	Nexthop         string
	DestinationCIDR string
}
