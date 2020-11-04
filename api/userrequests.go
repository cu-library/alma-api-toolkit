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

// UserRequest stores data about a user request on items.
// https://developers.exlibrisgroup.com/alma/apis/docs/xsd/rest_user_request.xsd/
type UserRequest struct {
	ID      string `xml:"request_id"`
	Type    string `xml:"request_type"`
	SubType string `xml:"request_sub_type"`
	// Member is an optional field for the 'originating' item member.
	Member Member `xml:"-"`
}

// MatchTypeSubType returns true if rtype is empty or matches the request's Type
// and the subType is empty or matches the request's SubType.
func (u UserRequest) MatchTypeSubType(rtype, subType string) bool {
	if (rtype == "" || rtype == u.Type) && (subType == "" || subType == u.SubType) {
		return true
	}
	return false
}

// UserRequests stores data about user requests on an item.
// This is a little different than []UserRequest for XML unmarshalling.
// https://developers.exlibrisgroup.com/alma/apis/docs/xsd/rest_user_requests.xsd/
type UserRequests struct {
	XMLName      xml.Name      `xml:"user_requests"` // The XML root element must have the name "item" or else Unmarshal returns an error.
	UserRequests []UserRequest `xml:"user_request"`
}

// ItemMembersUserRequests returns user requests on item members.
func (c Client) ItemMembersUserRequests(ctx context.Context, members []Member) (requests []UserRequest, errs []error) {
	ctx, cancel, em, om, jobs, wg, bar := StartConcurrent(ctx, len(members), "Getting user requests")
	defer cancel()
	for _, member := range members {
		member := member // avoid closure refering to wrong value
		jobs <- func() {
			itemRequests, err := c.ItemMemberUserRequests(ctx, member)
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
				requests = append(requests, itemRequests...)
			}
		}
		// Ignore the possible error returned by the progress bar.
		_ = bar.Add(1)
	}
	close(jobs)
	wg.Wait()
	return requests, errs
}

// ItemMemberUserRequests returns user requests on a item member.
func (c Client) ItemMemberUserRequests(ctx context.Context, member Member) (requests []UserRequest, err error) {
	url, err := url.Parse(member.Link + "/requests")
	if err != nil {
		return requests, err
	}
	r, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return requests, err
	}
	body, err := c.Do(ctx, r)
	if err != nil {
		return requests, err
	}
	returnedRequests := UserRequests{}
	err = xml.Unmarshal(body, &returnedRequests)
	if err != nil {
		return requests, fmt.Errorf("unmarshalling user requests XML failed: %w\n%v", err, string(body))
	}
	for i := range returnedRequests.UserRequests {
		returnedRequests.UserRequests[i].Member = member
	}
	return returnedRequests.UserRequests, nil
}

// UserRequestsCancel cancels user requests.
func (c Client) UserRequestsCancel(ctx context.Context, requests []UserRequest) (cancelled []UserRequest, errs []error) {
	ctx, cancel, em, om, jobs, wg, bar := StartConcurrent(ctx, len(requests), "Cancelling user requests")
	defer cancel()
	for _, request := range requests {
		request := request // avoid closure refering to wrong value
		jobs <- func() {
			err := c.UserRequestCancel(ctx, request)
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
				cancelled = append(cancelled, request)
			}
		}
		// Ignore the possible error returned by the progress bar.
		_ = bar.Add(1)
	}
	StopConcurrent(jobs, wg)
	return cancelled, errs
}

// UserRequestCancel cancels a user request.
func (c Client) UserRequestCancel(ctx context.Context, request UserRequest) error {
	if request.Member.Link == "" {
		return fmt.Errorf("the user request did not have an associated member link")
	}
	url, err := url.Parse(request.Member.Link + "/requests/" + request.ID)
	if err != nil {
		return err
	}
	r, err := http.NewRequest("DELETE", url.String(), nil)
	if err != nil {
		return err
	}
	_, err = c.Do(ctx, r)
	if err != nil {
		return err
	}
	return nil
}
