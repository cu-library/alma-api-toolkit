// Copyright 2020 Carleton University Library.
// All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.txt file.

// Package api provides an HTTP client which works with the Alma API.
package api

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
)

// Department stores data about a location within a library or institution where a service is performed.
// https://developers.exlibrisgroup.com/alma/apis/docs/xsd/rest_department.xsd/
type Department struct {
	Code string `xml:"code"`
	Name string `xml:"name"`
	Type struct {
		Text string `xml:",chardata"`
		Desc string `xml:"desc,attr"`
	} `xml:"type"`
	WorkDays string `xml:"work_days"`
	Printer  struct {
		Text string `xml:",chardata"`
		Desc string `xml:"desc,attr"`
	} `xml:"printer"`
	Owner struct {
		Text string `xml:",chardata"`
		Desc string `xml:"desc,attr"`
	} `xml:"owner"`
	ServedLibraries struct {
		Library []struct {
			Text string `xml:",chardata"`
			Desc string `xml:"desc,attr"`
		} `xml:"library"`
	} `xml:"served_libraries"`
	// Contact Info is TODO
	//ContactInfo struct {
	//	Addresses string `xml:"addresses"`
	//	Emails    string `xml:"emails"`
	//	Phones    string `xml:"phones"`
	//} `xml:"contact_info"`
	Operators struct {
		Operator []struct {
			Text      string `xml:",chardata"`
			Link      string `xml:"link,attr"`
			PrimaryID string `xml:"primary_id"`
			FullName  string `xml:"full_name"`
		} `xml:"operator"`
	} `xml:"operators"`
	Description string `xml:"description"`
}

// Departments stores data for all Departments configured for the Institution.
// This is a little different than []Department for XML unmarshalling.
// https://developers.exlibrisgroup.com/alma/apis/docs/xsd/rest_departments.xsd/
type Departments struct {
	XMLName          xml.Name     `xml:"departments"`
	Text             string       `xml:",chardata"`
	TotalRecordCount string       `xml:"total_record_count,attr"`
	Departments      []Department `xml:"department"`
}

// Departments returns the Departments configured for the Institution.
func (c Client) Departments(ctx context.Context) (departments Departments, err error) {
	r, err := http.NewRequest("GET", "/almaws/v1/conf/departments", nil)
	if err != nil {
		return departments, err
	}
	body, err := c.Do(ctx, r)
	if err != nil {
		return departments, err
	}
	err = xml.Unmarshal(body, &departments)
	if err != nil {
		return departments, fmt.Errorf("unmarshalling departments XML failed: %w\n%v", err, string(body))
	}
	return departments, nil
}
