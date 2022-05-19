//go:build ignore
// +build ignore

//go:generate go run iana_gen.go

// this program generates BGP-relevant IANA constants by reading IANA
// registries. Original inspiration for this generation technique stemmed from
// golang.org/x/net/internal/iana
package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"go/format"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var registries = []struct {
	url     string
	parseFn func(io.Writer, io.Reader) error
}{
	{
		url:     "https://www.iana.org/assignments/capability-codes/capability-codes.xml",
		parseFn: parseCapabilityRegistry,
	},
	{
		url:     "https://www.iana.org/assignments/address-family-numbers/address-family-numbers.xml",
		parseFn: parseAFIRegistry,
	},
	{
		url:     "https://www.iana.org/assignments/safi-namespace/safi-namespace.xml",
		parseFn: parseSAFIRegistry,
	},
}

func main() {
	var buf bytes.Buffer
	buf.WriteString("// go generate iana_gen.go\n")
	buf.WriteString("// Code generated by the command above; DO NOT EDIT.\n\n")
	buf.WriteString("package corebgp\n\n")
	client := http.Client{
		Timeout: time.Second * 10,
	}
	for _, r := range registries {
		resp, err := client.Get(r.url)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error retrieving %s: %v\n", r.url, err)
			os.Exit(1)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stderr, "got non-200 status (%d) for %s\n",
				resp.StatusCode, r.url)
			os.Exit(1)
		}
		err = r.parseFn(&buf, resp.Body)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error parsing resp from %s: %v\n", r.url,
				err)
			os.Exit(1)
		}
		buf.WriteString("\n")
	}
	b, err := format.Source(buf.Bytes())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error formatting source: %v\n", err)
		os.Exit(1)
	}
	err = os.WriteFile("iana_const.go", b, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error writing file: %v\n", err)
		os.Exit(1)
	}
}

type constRecord struct {
	originalName string
	name         string
	value        int
}

type capabilityRegistry struct {
	XMLName    xml.Name `xml:"registry"`
	Title      string   `xml:"title"`
	Updated    string   `xml:"updated"`
	Registries []struct {
		Title   string `xml:"title"`
		Records []struct {
			Value       string `xml:"value"`
			Description string `xml:"description"`
		} `xml:"record"`
	} `xml:"registry"`
}

func (c *capabilityRegistry) escape() []constRecord {
	constRecords := make([]constRecord, 0)
	for _, registry := range c.Registries {
		if registry.Title != "Capability Codes" {
			continue
		}
		sr := strings.NewReplacer(
			" for BGP-4", "",
			" Capability", "",
			" ", "_",
			"-", "_",
		)
		for _, record := range registry.Records {
			if strings.Contains(record.Description, "Reserved") ||
				strings.Contains(record.Description, "deprecated") ||
				strings.Contains(record.Description, "Deprecated") {
				continue
			}
			value, err := strconv.ParseUint(record.Value, 10, 8)
			if err != nil {
				continue
			}
			cr := constRecord{
				originalName: record.Description,
				value:        int(value),
			}
			s := record.Description
			switch s {
			case "Multiprotocol Extensions for BGP-4":
				cr.name = "MP_EXTENSIONS"
			case "BGP Extended Message":
				cr.name = "EXT_MESSSAGE"
			case "BGP Role":
				cr.name = "ROLE"
			case "Support for 4-octet AS number capability":
				cr.name = "FOUR_OCTET_AS"
			case "Support for Dynamic Capability (capability specific)":
				cr.name = "DYNAMIC"
			case "Multisession BGP Capability":
				cr.name = "MULTISESSION"
			case "Long-Lived Graceful Restart (LLGR) Capability":
				cr.name = "LLGR"
			case "Routing Policy Distribution":
				cr.name = "ROUTING_POLICY_DIST"
			default:
				s = strings.TrimSpace(s)
				s = sr.Replace(s)
				cr.name = strings.ToUpper(s)
			}
			constRecords = append(constRecords, cr)
		}
	}
	return constRecords
}

func parseCapabilityRegistry(w io.Writer, r io.Reader) error {
	c := capabilityRegistry{}
	dec := xml.NewDecoder(r)
	err := dec.Decode(&c)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "// %s, Updated: %s\n", c.Title, c.Updated)
	fmt.Fprint(w, "const(\n")
	for _, cr := range c.escape() {
		fmt.Fprintf(w, "CAP_%s uint8 = %d", cr.name, cr.value)
		fmt.Fprintf(w, "// %s\n", cr.originalName)
	}
	fmt.Fprint(w, ")\n")
	return nil
}

type afiRegistry struct {
	XMLName  xml.Name `xml:"registry"`
	Title    string   `xml:"title"`
	Updated  string   `xml:"updated"`
	Registry struct {
		Title   string `xml:"title"`
		Records []struct {
			Value       string `xml:"value"`
			Description string `xml:"description"`
		} `xml:"record"`
	} `xml:"registry"`
}

func (a *afiRegistry) escape() []constRecord {
	constRecords := make([]constRecord, 0)
	sr := strings.NewReplacer(
		"Identifier", "ID",
		" ", "_",
		".", "",
		"-", "_",
	)
	for _, record := range a.Registry.Records {
		if strings.Contains(record.Description, "Reserved") ||
			strings.Contains(record.Description, "Unassigned") {
			continue
		}
		value, err := strconv.ParseUint(record.Value, 10, 16)
		if err != nil {
			continue
		}
		cr := constRecord{
			originalName: record.Description,
			value:        int(value),
		}
		s := record.Description
		switch s {
		case "IP (IP version 4)":
			cr.name = "IPV4"
		case "IP6 (IP version 6)":
			cr.name = "IPV6"
		case "E.164 with NSAP format subaddress":
			cr.name = "E164_WITH_NSAP_SUBADDR"
		case "XTP over IP version 4":
			cr.name = "XTP_OVER_IPV4"
		case "XTP over IP version 6":
			cr.name = "XTP_OVER_IPV6"
		case "XTP native mode XTP":
			cr.name = "XTP_NATIVE"
		case "Fibre Channel World-Wide Port Name":
			cr.name = "FIBRE_CHANNEL_WWPN"
		case "Fibre Channel World-Wide Node Name":
			cr.name = "FIBRE_CHANNEL_WWNN"
		case "AFI for L2VPN information":
			cr.name = "L2VPN_INFO"
		case "MT IP: Multi-Topology IP version 4":
			cr.name = "MT_IPV4"
		case "MT IPv6: Multi-Topology IP version 6":
			cr.name = "MT_IPV6"
		case "LISP Canonical Address Format (LCAF)":
			cr.name = "LCAF"
		case "MAC/24":
			cr.name = "MAC_FINAL_24_BITS"
		case "MAC/40":
			cr.name = "MAC_FINAL_40_BITS"
		case "IPv6/64":
			cr.name = "IPV6_INITIAL_64_BITS"
		case "Routing Policy AFI":
			cr.name = "ROUTING_POLICY"
		case "Universally Unique Identifier (UUID)":
			cr.name = "UUID"
		default:
			n := strings.Index(s, "(")
			if n > 0 {
				s = s[:n]
			}
			n = strings.Index(s, ":")
			if n > 0 {
				s = s[:n]
			}
			s = strings.TrimSpace(s)
			s = sr.Replace(s)
			cr.name = strings.ToUpper(s)
		}
		constRecords = append(constRecords, cr)
	}
	return constRecords
}

func parseAFIRegistry(w io.Writer, r io.Reader) error {
	a := afiRegistry{}
	dec := xml.NewDecoder(r)
	err := dec.Decode(&a)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "// %s, Updated: %s\n", a.Title, a.Updated)
	fmt.Fprint(w, "const(\n")
	for _, afc := range a.escape() {
		fmt.Fprintf(w, "AFI_%s uint16 = %d", afc.name, afc.value)
		fmt.Fprintf(w, "// %s\n", afc.originalName)
	}
	fmt.Fprint(w, ")\n")
	return nil
}

type safiRegistry struct {
	XMLName  xml.Name `xml:"registry"`
	Title    string   `xml:"title"`
	Updated  string   `xml:"updated"`
	Registry struct {
		Title   string `xml:"title"`
		Records []struct {
			Value       string `xml:"value"`
			Description string `xml:"description"`
		} `xml:"record"`
	} `xml:"registry"`
}

func (a *safiRegistry) escape() []constRecord {
	constRecords := make([]constRecord, 0)
	sr := strings.NewReplacer(
		" SAFI", "",
		"Flow Specification", "FLOWSPEC",
		" ", "_",
		".", "",
		"-", "_",
		"/", "",
	)
	for _, record := range a.Registry.Records {
		if strings.Contains(record.Description, "Reserved") ||
			strings.Contains(record.Description, "Unassigned") ||
			strings.Contains(record.Description, "OBSOLETE") {
			continue
		}
		value, err := strconv.ParseUint(record.Value, 10, 8)
		if err != nil {
			continue
		}
		cr := constRecord{
			originalName: record.Description,
			value:        int(value),
		}
		s := record.Description
		switch s {
		case "Network Layer Reachability Information used     \nfor unicast forwarding":
			cr.originalName = "Network Layer Reachability Information used for unicast forwarding"
			cr.name = "UNICAST"
		case "Network Layer Reachability Information used     \nfor multicast forwarding":
			cr.originalName = "Network Layer Reachability Information used for multicast forwarding"
			cr.name = "MULTICAST"
		case "Network Layer Reachability Information (NLRI)   \nwith MPLS Labels":
			cr.originalName = "Network Layer Reachability Information (NLRI) with MPLS Labels"
			cr.name = "MPLS"
		case "Network Layer Reachability Information used     \nfor Dynamic Placement of Multi-Segment Pseudowires":
			cr.originalName = "Network Layer Reachability Information used for Dynamic Placement of Multi-Segment Pseudowires"
			cr.name = "DYN_PLACEMENT_MULTI_SEGMENT_PW"
		case "Virtual Private LAN Service (VPLS)":
			cr.name = "VPLS"
		case "Layer-1 VPN auto-discovery information":
			cr.name = "LAYER_1_VPN_AUTO_DISCOVERY_INFO"
		case "MPLS-labeled VPN address":
			cr.name = "MPLS_LABELED_VPN_ADDR"
		case "Multicast for BGP/MPLS IP Virtual Private       \nNetworks (VPNs)":
			cr.originalName = "Multicast for BGP/MPLS IP Virtual Private Networks (VPNs)"
			cr.name = "MULTICAST_BGP_MPLS_IP_VPNS"
		default:
			n := strings.Index(s, "(")
			if n > 0 {
				s = s[:n]
			}
			n = strings.Index(s, ":")
			if n > 0 {
				s = s[:n]
			}
			s = strings.TrimSpace(s)
			cr.name = strings.ToUpper(sr.Replace(s))
		}
		constRecords = append(constRecords, cr)
	}
	return constRecords
}

func parseSAFIRegistry(w io.Writer, r io.Reader) error {
	s := safiRegistry{}
	dec := xml.NewDecoder(r)
	err := dec.Decode(&s)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "// %s, Updated: %s\n", s.Title, s.Updated)
	fmt.Fprint(w, "const(\n")
	for _, cr := range s.escape() {
		fmt.Fprintf(w, "SAFI_%s uint8 = %d", cr.name, cr.value)
		fmt.Fprintf(w, "// %s\n", cr.originalName)
	}
	fmt.Fprint(w, ")\n")
	return nil
}