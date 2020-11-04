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
)

// CodeTable stores data about codes and their related descriptions.
// https://developers.exlibrisgroup.com/alma/apis/docs/xsd/rest_code_table.xsd/
type CodeTable struct {
	XMLName     xml.Name `xml:"code_table"`
	Name        string   `xml:"name"`
	Description string   `xml:"description"`
	SubSystem   struct {
		Text string `xml:",chardata"`
		Desc string `xml:"desc,attr"`
	} `xml:"sub_system"`
	PatronFacing string `xml:"patron_facing"`
	Language     struct {
		Text string `xml:",chardata"`
		Desc string `xml:"desc,attr"`
	} `xml:"language"`
	Scope struct {
		InstitutionID struct {
			Text string `xml:",chardata"`
			Desc string `xml:"desc,attr"`
		} `xml:"institution_id"`
		LibraryID struct {
			Text string `xml:",chardata"`
			Desc string `xml:"desc,attr"`
		} `xml:"library_id"`
	} `xml:"scope"`
	Rows []struct {
		Text        string `xml:",chardata"`
		Code        string `xml:"code"`
		Description string `xml:"description"`
		Default     string `xml:"default"`
		Enabled     string `xml:"enabled"`
	} `xml:"rows>row"`
}

// CodeTables returns all known code tables.
func (c Client) CodeTables(ctx context.Context) (tables []CodeTable, errs []error) {
	names := []string{
		"accessionPlacementsOptions",
		"AcqItemSourceType",
		"AcquisitionMethod",
		"ActiveResourcesTypes",
		"AddNewUserOptions",
		"AdminURIType",
		"ARTEmailDeliveryKeywords",
		"ARTEmailQueriesKeywords",
		"ARTEmailServiceKeywords",
		"AssertionCodes",
		"BaseStatus",
		"BLDSSDigitalFormats",
		"BooleanYesNo",
		"CalendarRecordStatuses",
		"CalendarRecordsTypes",
		"CallNumberType",
		"CampusListSearchableColumns",
		"CatalogerLevel",
		"CitationAttributes",
		"CitationAttributesTypes",
		"CitationCopyRights",
		"CollectionAccessType",
		"ContentStructureStatus",
		"CounterPlatform",
		"CountryCodes",
		"CourseTerms",
		"CoverageInUse",
		"crossRefEnabled",
		"CrossRefSupported",
		"Currency_CT",
		"DaysOfWeek",
		"DigitalRepresentationBaseStatus",
		"EDINamingConvention",
		"EdiPreference",
		"EdiType",
		"ElectronicBaseStatus",
		"electronicMaterialType",
		"ElectronicPortfolioBaseStatus",
		"ExpiryType",
		"ExternalSystemTypes",
		"FineFeeTransactionType",
		"FTPMode",
		"FTPSend",
		"FundType",
		"Genders",
		"GroupProxyEnabled",
		"HFrUserFinesFees.fineFeeStatus",
		"HFrUserFinesFees.fineFeeType",
		"HFrUserRoles.roleType",
		"HfundLedger.status",
		"HFundsTransactionItem.reportingCode",
		"HItemLoan.processStatus",
		"HLicense.status",
		"HLicense.type",
		"HLocation.locationType",
		"HPaTaskChain.businessEntity",
		"HPaTaskChain.type",
		"ImplementedAuthMethod",
		"IntegrationTypes",
		"InvoiceApprovalStatus",
		"InvoiceCreationForm",
		"InvoiceLineStatus",
		"InvoiceLinesTypes",
		"InvoiceStatus",
		"IpAddressRegMethod",
		"isAggregator",
		"IsFree",
		"ItemPhysicalCondition",
		"ItemPolicy",
		"JobsApiJobTypes",
		"jobScheduleNames",
		"JobTitles",
		"LevelOfService",
		"LibraryNoticesOptInDisplay",
		"LicenseReviewStatuses",
		"LicenseStorageLocation",
		"LicenseTerms",
		"LicenseTermsAndTypes",
		"LinkingLevel",
		"LinkResolverPlugin",
		"marcLanguage",
		"Months",
		"MovingWallOperator",
		"NoteTypes",
		"OwnerHierarchy",
		"PartnerSystemTypes",
		"PaymentMethod",
		"PaymentStatus",
		"PhysicalMaterialType",
		"PhysicalReadingListCitationTypes",
		"POLineStatus",
		"PortfolioAccessType",
		"PPRSourceType",
		"PR_CitationType",
		"PR_RejectReasons",
		"PR_RequestedFormat",
		"PROCESSTYPE",
		"provenanceCodes",
		"PurchaseRequestStatus",
		"PurchaseType",
		"ReadingListCitationSecondaryTypes",
		"ReadingListCitationTypes",
		"ReadingListRLStatuses",
		"ReadingListStatuses",
		"ReadingListVisibilityStatuses",
		"RecurrenceType",
		"ReminderStatuses",
		"ReminderTypes",
		"RenewalCycle",
		"representationEntityType",
		"RepresentationUsageType",
		"RequestFormats",
		"RequestOptions",
		"ResourceSharingCopyrightsStatus",
		"ResourceSharingLanguages",
		"ResourceSharingRequestSendMethod",
		"SecondReportingCode",
		"ServiceType",
		"SetContentType",
		"SetPrivacy",
		"SetStatus",
		"SetType",
		"ShippingMethod",
		"Sub Systems",
		"SystemJobReportAlertMessage",
		"systemJobStatus",
		"TagTypes",
		"ThirdReportingCode",
		"UsageStatsDeliveryMethod",
		"UsageStatsFormat",
		"UsageStatsFrequency",
		"UserAddressTypes",
		"UserBlockDescription",
		"UserBlockTypes",
		"UserEmailTypes",
		"UserGroups",
		"UserIdentifierTypes",
		"UserPhoneTypes",
		"UserPreferredLanguage",
		"UserRoleStatus",
		"UserStatCategories",
		"UserStatisticalTypes",
		"UserUserType",
		"UserWebAddressTypes",
		"VATType",
		"VendorReferenceNumberType",
		"VendorSearchStatusFilter",
		"WebhookEvents",
		"WebhooksActionType",
		"WorkbenchPaymentMethod",
	}
	ctx, cancel, em, om, jobs, wg, bar := StartConcurrent(ctx, len(names), "Getting code tables")
	defer cancel()
	for _, name := range names {
		name := name // avoid closure refering to wrong value
		jobs <- func() {
			table, err := c.CodeTable(ctx, name)
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
				tables = append(tables, table)
			}
		}
		// Ignore the possible error returned by the progress bar.
		_ = bar.Add(1)
	}
	StopConcurrent(jobs, wg)
	return tables, errs
}

// CodeTable returns a code table.
func (c Client) CodeTable(ctx context.Context, name string) (table CodeTable, err error) {
	r, err := http.NewRequest("GET", "/almaws/v1/conf/code-tables/"+name, nil)
	if err != nil {
		return table, err
	}
	body, err := c.Do(ctx, r)
	if err != nil {
		return table, err
	}
	err = xml.Unmarshal(body, &table)
	if err != nil {
		return table, fmt.Errorf("unmarshalling code table XML failed: %w\n%v", err, string(body))
	}
	return table, nil
}
