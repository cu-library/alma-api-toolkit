// Copyright 2020 Carleton University Library.
// All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.txt file.

// Package api provides an HTTP client which works with the Alma API.
package api

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// DefaultCircDesk is the default circulation desk code used when scanning an item in.
const DefaultCircDesk = "DEFAULT_CIRC_DESK"

// Item stores data about an item.
// https://developers.exlibrisgroup.com/alma/apis/docs/xsd/rest_sets.xsd/
type Item struct {
	XMLName    xml.Name `xml:"item"` //the XML root element must have the name "item" or else Unmarshal returns an error.
	MMSID      string   `xml:"bib_data>mms_id"`
	Title      string   `xml:"bib_data>title"`
	Author     string   `xml:"bib_data>author"`
	CallNumber string   `xml:"holding_data>call_number"`
	Barcode    string   `xml:"item_data>barcode"`
}

// ItemMembersScanIn scans members in. The members must be from a set with content ITEM.
func (c Client) ItemMembersScanIn(ctx context.Context, members []Member, circdesk, library string) (scannedIn []Item, errs []error) {
	ctx, cancel, em, om, jobs, wg, bar := StartConcurrent(ctx, len(members), "Scanning items in")
	defer cancel()
	for _, member := range members {
		member := member // avoid closure refering to wrong value
		jobs <- func() {
			item, err := c.ItemMemberScanIn(ctx, member, circdesk, library)
			if err != nil {
				var over *ThresholdReachedError
				if errors.As(err, &over) {
					// We've reached the threshold, cancel the context.
					cancel()
				}
				em.Lock()
				defer em.Unlock()
				errs = append(errs, err)
			} else {
				om.Lock()
				defer om.Unlock()
				scannedIn = append(scannedIn, item)
			}
		}
		// Ignore the possible error returned by the progress bar.
		_ = bar.Add(1)
	}
	StopConcurrent(jobs, wg)
	return scannedIn, errs
}

// ItemMemberScanIn POSTs the scan operation on an item member.
func (c Client) ItemMemberScanIn(ctx context.Context, member Member, circdesk, library string) (item Item, err error) {
	url, err := url.Parse(member.Link)
	if err != nil {
		return item, err
	}
	q := url.Query()
	q.Set("op", "scan")
	q.Set("register_in_house_use", "false")
	q.Set("circ_desk", circdesk)
	q.Set("library", library)
	url.RawQuery = q.Encode()
	r, err := http.NewRequest("POST", url.String(), nil)
	if err != nil {
		return item, err
	}
	body, err := c.Do(ctx, r)
	if err != nil {
		return item, err
	}
	err = xml.Unmarshal(body, &item)
	if err != nil {
		return item, fmt.Errorf("unmarshalling item XML failed: %w\n%v", err, string(body))
	}
	return item, err
}
