package stadis

import "time"

var DefaultConfig = []byte(`
{
	"DcDefault":{"Latency":100000000},
	"RackDefault":{"Latency":10000000},
	"HostDefault":{"Latency":1000000},
	"DataCenters":[
		{
			"Name":"animal",
			"Racks":[
				{
					"Name":"land",
					"Hosts":[{"Name":"tiger"},{"Name":"lion"},{"Name":"wolf"}]
				},
				{
					"Name":"sea",
					"Hosts":[{"Name":"shark"},{"Name":"whale"},{"Name":"cod"}]
				},
				{
					"Name":"air",
					"Hosts":[{"Name":"eagle"},{"Name":"crow"},{"Name":"owl"}]
				}
			]
		},
		{
			"Name":"plant",
			"Racks":[
				{
					"Name":"fruit",
					"Hosts":[{"Name":"apple"},{"Name":"pear"},{"Name":"grape"}]
				},
				{
					"Name":"crop",
					"Hosts":[{"Name":"corn"},{"Name":"rice"},{"Name":"wheat"}]
				},
				{
					"Name":"flower",
					"Hosts":[{"Name":"rose"},{"Name":"lily"},{"Name":"lotus"}]
				}
			]
		},
		{
			"Name":"matter",
			"Racks":[
				{
					"Name":"metal",
					"Hosts":[{"Name":"gold"},{"Name":"silver"},{"Name":"iron"}]
				},
				{
					"Name":"gem",
					"Hosts":[{"Name":"ruby"},{"Name":"ivory"},{"Name":"pearl"}]
				},
				{
					"Name":"liquid",
					"Hosts":[{"Name":"water"},{"Name":"oil"},{"Name":"wine"}]
				}
			]
		}
	]
}
`)

type ConnState struct {
	Latency time.Duration //the sleep time before write data to the connection.
	OK      bool          //If not ok, local process should not write data to the connection.
}

type NodeState struct {
	Latency      time.Duration
	InternalDown bool
	ExternalDown bool
}

type Config struct {
	DcDefault   *NodeState
	RackDefault *NodeState
	HostDefault *NodeState
	DataCenters []*DataCenter
}

type DataCenter struct {
	RackDefault *NodeState
	HostDefault *NodeState
	Name        string
	Racks       []*Rack
	*NodeState
}

type Rack struct {
	HostDefault *NodeState
	Name        string
	Hosts       []*Host
	*NodeState
}

type Host struct {
	Name  string
	Ports []int
	*NodeState
}
