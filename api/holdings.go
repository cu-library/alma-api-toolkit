// Copyright 2020 Carleton University Library.
// All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.txt file.

// Package api provides an HTTP client which works with the Alma API.
package api

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// HoldingListMember stores data about a holding record when it's returned from the Holdings list.
// HoldingListMember != Holding
// https://developers.exlibrisgroup.com/alma/apis/docs/xsd/rest_holdings.xsd
type HoldingListMember struct {
	Link      string `xml:"link,attr"`
	HoldingID string `xml:"holding_id"`
	Library   struct {
		Text string `xml:",chardata"`
		Desc string `xml:"desc,attr"`
	} `xml:"library"`
	Location struct {
		Text string `xml:",chardata"`
		Desc string `xml:"desc,attr"`
	} `xml:"location"`
	CallNumber             string `xml:"call_number"`
	SuppressFromPublishing string `xml:"suppress_from_publishing"`
}

// Holdings stores data about holdings under a bib record.
// https://developers.exlibrisgroup.com/alma/apis/docs/xsd/rest_holdings.xsd/
type Holdings struct {
	XMLName            xml.Name            `xml:"holdings"`
	TotalRecordCount   string              `xml:"total_record_count,attr"`
	HoldingListMembers []HoldingListMember `xml:"holding"`
	BibData            struct {
		Link           string `xml:"link,attr"`
		MmsID          string `xml:"mms_id"`
		Title          string `xml:"title"`
		ISSN           string `xml:"issn"`
		NetworkNumbers struct {
			Text          string `xml:",chardata"`
			NetworkNumber string `xml:"network_number"`
		} `xml:"network_numbers"`
		Publisher string `xml:"publisher"`
	} `xml:"bib_data"`
}

// Holding stores data about a holding record.
// https://developers.exlibrisgroup.com/alma/apis/docs/xsd/rest_holding.xsd/
type Holding struct {
	XMLName                xml.Name `xml:"holding"`
	HoldingID              string   `xml:"holding_id"`
	CreatedBy              string   `xml:"created_by"`
	CreatedDate            string   `xml:"created_date"`
	OriginatingSystem      string   `xml:"originating_system"`
	OriginatingSystemID    string   `xml:"originating_system_id"`
	SuppressFromPublishing string   `xml:"suppress_from_publishing"`
	Record                 struct {
		Text         string `xml:",chardata"`
		Leader       string `xml:"leader"`
		Controlfield []struct {
			Text string `xml:",chardata"`
			Tag  string `xml:"tag,attr"`
		} `xml:"controlfield"`
		Datafield []struct {
			Text     string `xml:",chardata"`
			Ind1     string `xml:"ind1,attr"`
			Ind2     string `xml:"ind2,attr"`
			Tag      string `xml:"tag,attr"`
			Subfield []struct {
				Text string `xml:",chardata"`
				Code string `xml:"code,attr"`
			} `xml:"subfield"`
		} `xml:"datafield"`
	} `xml:"record"`
	// HoldingListMember is an optional field for the 'originating' holding list member.
	HoldingListMember HoldingListMember `xml:"-"`
}

// EightFiftyTwoSubHSubI returns the h and i parts of the 852, seperated with a space.
func (h Holding) EightFiftyTwoSubHSubI() (callnumber string) {
	for _, field := range h.Record.Datafield {
		if field.Tag == "852" {
			for _, sub := range field.Subfield {
				if sub.Code == "h" {
					callnumber = sub.Text
				}
			}
			for _, sub := range field.Subfield {
				if sub.Code == "i" {
					callnumber = callnumber + " " + sub.Text
				}
			}
		}
	}
	return callnumber
}

// BibMembersHoldingListMembers returns the holding list members for the members. The members must be from a set with content BIB_MMS.
func (c Client) BibMembersHoldingListMembers(ctx context.Context, members []Member) (holdingListMembers []HoldingListMember, errs []error) {
	ctx, cancel, em, om, jobs, wg, bar := StartConcurrent(ctx, len(members), "Getting holding list members")
	defer cancel()
	for _, member := range members {
		member := member // avoid closure refering to wrong value
		jobs <- func() {
			memberHoldingListMembers, err := c.BibMemberHoldingListMembers(ctx, member)
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
				holdingListMembers = append(holdingListMembers, memberHoldingListMembers...)
			}
		}
		// Ignore the possible error returned by the progress bar.
		_ = bar.Add(1)
	}
	StopConcurrent(jobs, wg)
	return holdingListMembers, errs
}

// BibMemberHoldingListMembers returns the holding list members for a member of type BIB_MMS.
func (c Client) BibMemberHoldingListMembers(ctx context.Context, member Member) (holdingListMembers []HoldingListMember, err error) {
	url, err := url.Parse(member.Link + "/holdings")
	if err != nil {
		return holdingListMembers, err
	}
	r, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return holdingListMembers, err
	}
	body, err := c.Do(ctx, r)
	if err != nil {
		return holdingListMembers, err
	}
	holdings := Holdings{}
	err = xml.Unmarshal(body, &holdings)
	if err != nil {
		return holdingListMembers, fmt.Errorf("unmarshalling holdings XML failed: %w\n%v", err, string(body))
	}
	return holdings.HoldingListMembers, nil
}

// HoldingListMembersToHoldings returns the holdings records refered to by a slice of holdings list members.
func (c Client) HoldingListMembersToHoldings(ctx context.Context, holdingListMembers []HoldingListMember) (holdings []Holding, errs []error) {
	ctx, cancel, em, om, jobs, wg, bar := StartConcurrent(ctx, len(holdingListMembers), "Getting holdings records")
	defer cancel()
	for _, holdingListMember := range holdingListMembers {
		holdingListMember := holdingListMember // avoid closure refering to wrong value
		jobs <- func() {
			holding, err := c.HoldingListMemberToHolding(ctx, holdingListMember)
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
				holdings = append(holdings, holding)
			}
		}
		// Ignore the possible error returned by the progress bar.
		_ = bar.Add(1)
	}
	StopConcurrent(jobs, wg)
	return holdings, errs
}

// HoldingListMemberToHolding returns the holding records refered to by a holding list member.
func (c Client) HoldingListMemberToHolding(ctx context.Context, holdingListMember HoldingListMember) (holding Holding, err error) {
	url, err := url.Parse(holdingListMember.Link)
	if err != nil {
		return holding, err
	}
	r, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return holding, err
	}
	body, err := c.Do(ctx, r)
	if err != nil {
		return holding, err
	}
	err = xml.Unmarshal(body, &holding)
	if err != nil {
		return holding, fmt.Errorf("unmarshalling holding XML failed: %w\n%v", err, string(body))
	}
	// Enrich the API record with the originating holding list member.
	holding.HoldingListMember = holdingListMember
	return holding, nil
}

// HoldingsUpdate PUTs the holdings back to the API.
func (c Client) HoldingsUpdate(ctx context.Context, holdings []Holding) (updatedHoldings []Holding, errs []error) {
	ctx, cancel, em, om, jobs, wg, bar := StartConcurrent(ctx, len(holdings), "Updating holdings records")
	defer cancel()
	for _, holding := range holdings {
		holding := holding // avoid closure refering to wrong value
		jobs <- func() {
			updated, err := c.HoldingUpdate(ctx, holding)
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
				updatedHoldings = append(updatedHoldings, updated)
			}
		}
		// Ignore the possible error returned by the progress bar.
		_ = bar.Add(1)
	}
	StopConcurrent(jobs, wg)
	return updatedHoldings, errs
}

// HoldingUpdate PUTs the holding back to the API.
func (c Client) HoldingUpdate(ctx context.Context, holding Holding) (updated Holding, err error) {
	url, err := url.Parse(holding.HoldingListMember.Link)
	if err != nil {
		return updated, err
	}
	holdingBytes, err := xml.Marshal(holding)
	if err != nil {
		return updated, fmt.Errorf("marshalling holding XML failed: %w", err)
	}
	r, err := http.NewRequest("PUT", url.String(), bytes.NewReader(holdingBytes))
	if err != nil {
		return updated, err
	}
	r.Header.Add("Content-Type", "application/xml")
	body, err := c.Do(ctx, r)
	if err != nil {
		return updated, err
	}
	err = xml.Unmarshal(body, &updated)
	if err != nil {
		return updated, fmt.Errorf("unmarshalling holding XML failed: %w\n%v", err, string(body))
	}
	// Enrich the API record with the originating holding list member.
	updated.HoldingListMember = holding.HoldingListMember
	return updated, nil
}
