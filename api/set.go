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
	"strconv"
	"strings"
)

// Set stores data about sets.
// https://developers.exlibrisgroup.com/alma/apis/docs/xsd/rest_set.xsd/
type Set struct {
	ID              string `xml:"id"`
	Name            string `xml:"name"`
	NumberOfMembers int    `xml:"number_of_members"`
	Type            string `xml:"type"`
	Content         string `xml:"content"`
	Link            string `xml:"link,attr"`
}

// Sets stores data about lists of sets as returned from the API.
// This is a little different than []Set for safer XML unmarshalling.
// https://developers.exlibrisgroup.com/alma/apis/docs/xsd/rest_sets.xsd/
type Sets struct {
	XMLName xml.Name `xml:"sets"` //the XML root element must have the name "sets" or else Unmarshal returns an error.
	Sets    []Set    `xml:"set"`
}

// Member stores data about members of sets.
// https://developers.exlibrisgroup.com/alma/apis/docs/xsd/rest_member.xsd/
type Member struct {
	ID   string `xml:"id"`
	Link string `xml:"link,attr"`
}

// Members stores data about set members.
// This is a little different than []Member for safer XML unmarshalling.
// https://developers.exlibrisgroup.com/alma/apis/docs/xsd/rest_members.xsd/
type Members struct {
	XMLName xml.Name `xml:"members"` //the XML root element must have the name "members" or else Unmarshal returns an error.
	Members []Member `xml:"member"`
}

// SetFromNameOrID returns the set when provided the name or ID.
func (c Client) SetFromNameOrID(ctx context.Context, name, ID string) (set Set, err error) {
	if name != "" {
		var err error
		ID, err = c.SetIDFromName(ctx, name)
		if err != nil {
			return set, fmt.Errorf("getting set ID from name failed: %w", err)
		}
	}
	set, err = c.SetFromID(ctx, ID)
	if err != nil {
		return set, fmt.Errorf("getting set from ID failed: %w", err)
	}
	return set, nil
}

// SetIDFromName returns the ID for the set with the given set name.
func (c Client) SetIDFromName(ctx context.Context, name string) (ID string, err error) {
	url := &url.URL{
		Path: "/almaws/v1/conf/sets",
	}
	q := url.Query()
	q.Set("q", "name~"+name)
	url.RawQuery = q.Encode()
	r, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return ID, err
	}
	body, err := c.Do(ctx, r)
	if err != nil {
		return ID, err
	}
	sets := Sets{}
	err = xml.Unmarshal(body, &sets)
	if err != nil {
		return ID, fmt.Errorf("unmarshalling sets XML failed: %w\n%v", err, string(body))
	}
	for _, set := range sets.Sets {
		if strings.TrimSpace(set.Name) == strings.TrimSpace(name) {
			return set.ID, nil
		}
	}
	return ID, fmt.Errorf("no set with name '%v' found", name)
}

// SetFromID returns the set with the given set ID.
func (c Client) SetFromID(ctx context.Context, ID string) (set Set, err error) {
	r, err := http.NewRequest("GET", "/almaws/v1/conf/sets/"+ID, nil)
	if err != nil {
		return set, err
	}
	body, err := c.Do(ctx, r)
	if err != nil {
		return set, err
	}
	err = xml.Unmarshal(body, &set)
	if err != nil {
		return set, fmt.Errorf("unmarshalling set XML failed: %w\n%v", err, string(body))
	}
	return set, nil
}

// SetMembers returns the members of a set.
func (c Client) SetMembers(ctx context.Context, set Set) (members []Member, errs []error) {
	// With a map, we can ensure we don't get duplicate members from the API.
	membersMap := map[string]Member{}
	offsets := set.NumberOfMembers / LimitParam
	// If there's a remainder, we need the extra offset.
	if set.NumberOfMembers%LimitParam != 0 {
		offsets++
	}
	ctx, cancel, em, om, jobs, wg, bar := StartConcurrent(ctx, offsets, "Getting set members")
	defer cancel()
	for i := 0; i < offsets; i++ {
		offset := i * LimitParam
		jobs <- func() {
			members, err := c.setMembersLimitOffset(ctx, set, LimitParam, offset)
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
				for _, member := range members {
					membersMap[member.ID] = member
				}
			}
		}
		// Ignore the possible error returned by the progress bar.
		_ = bar.Add(1)
	}
	StopConcurrent(jobs, wg)
	if len(membersMap) != set.NumberOfMembers {
		return members, append(errs, fmt.Errorf("%v members found for set with size %v", len(membersMap), set.NumberOfMembers))
	}
	members = make([]Member, 0, len(membersMap)) // Small optimzation, we already know the size of the underlying array we need.
	for _, member := range membersMap {
		members = append(members, member)
	}
	return members, errs
}

// setMembersLimitOffset returns the members of the set at the particular offset.
func (c Client) setMembersLimitOffset(ctx context.Context, set Set, limit, offset int) (members []Member, err error) {
	url, err := url.Parse(set.Link + "/members")
	if err != nil {
		return members, err
	}
	q := url.Query()
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", strconv.Itoa(offset))
	url.RawQuery = q.Encode()
	r, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return members, err
	}
	body, err := c.Do(ctx, r)
	if err != nil {
		return members, err
	}
	returnedMembers := Members{}
	err = xml.Unmarshal(body, &returnedMembers)
	if err != nil {
		return members, fmt.Errorf("unmarshalling set XML failed: %w\n%v", err, string(body))
	}
	return returnedMembers.Members, nil
}
