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

// Library stores data about a library in Alma, which represents a physical library in the institution, which gives library services.
// https://developers.exlibrisgroup.com/alma/apis/docs/xsd/rest_library.xsd/
type Library struct {
	Link            string `xml:"link,attr"`
	Code            string `xml:"code"`
	Path            string `xml:"path"`
	Name            string `xml:"name"`
	Description     string `xml:"description"`
	ResourceSharing string `xml:"resource_sharing"`
	Campus          struct {
		Text string `xml:",chardata"`
		Desc string `xml:"desc,attr"`
	} `xml:"campus"`
	Proxy           string `xml:"proxy"`
	DefaultLocation struct {
		Text string `xml:",chardata"`
		Desc string `xml:"desc,attr"`
	} `xml:"default_location"`
}

// Libraries stores data about libraries configured for the Institution.
// This is a little different than []Library for XML unmarshalling.
// https://developers.exlibrisgroup.com/alma/apis/docs/xsd/rest_libraries.xsd/
type Libraries struct {
	XMLName   xml.Name  `xml:"libraries"`
	Libraries []Library `xml:"library"`
}

// Libraries returns the Libraries configured for the Institution.
func (c Client) Libraries(ctx context.Context) (libraries Libraries, err error) {
	r, err := http.NewRequest("GET", "/almaws/v1/conf/libraries", nil)
	if err != nil {
		return libraries, err
	}
	body, err := c.Do(ctx, r)
	if err != nil {
		return libraries, err
	}
	err = xml.Unmarshal(body, &libraries)
	if err != nil {
		return libraries, fmt.Errorf("unmarshalling libraries XML failed: %w\n%v", err, string(body))
	}
	return libraries, nil
}
