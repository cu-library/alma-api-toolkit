// Copyright 2020 Carleton University Library.
// All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.txt file.

package main

import (
	"encoding/xml"
	"flag"
	"fmt"
	"net/http"
	"sync"
)

func (m SubcommandMap) addConfLibrariesDepartmentsCodeTables() {
	fs := flag.NewFlagSet("conf-libaries-departments-code-tables", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "  Print the output of the library and departments endpoints, and the known code tables.")
		fmt.Fprintln(flag.CommandLine.Output(), "  The list of known code tables comes from")
		fmt.Fprintln(flag.CommandLine.Output(), "  https://developers.exlibrisgroup.com/blog/almas-code-tables-api-list-of-code-tables/")
		fmt.Fprintln(flag.CommandLine.Output(), "  This command is meant to help run other subcommands which sometimes need a particular code")
		fmt.Fprintln(flag.CommandLine.Output(), "  from a code table or the code for a library or department.")
	}
	m[fs.Name()] = &Subcommand{
		ReadAccess: []string{"/almaws/v1/conf"},
		FlagSet:    fs,
		Run: func(requester Requester) []error {
			libraries, err := getLibraries(requester)
			if err != nil {
				return []error{err}
			}
			fmt.Println("Libraries:")
			for _, library := range libraries.Libraries {
				fmt.Printf("%v (%v)\n", library.Code, library.Name)
				fmt.Printf("Description: %v\n", library.Description)
				fmt.Printf("Resource Sharing: %v\n", library.ResourceSharing)
				fmt.Printf("Campus: %v (%v)\n", library.Campus.Text, library.Campus.Desc)
				fmt.Printf("Proxy: %v\n", library.Proxy)
				fmt.Printf("Default Location: %v (%v)\n", library.DefaultLocation.Text, library.DefaultLocation.Desc)
				fmt.Println()
			}
			departments, err := getDepartments(requester)
			if err != nil {
				return []error{err}
			}
			fmt.Println("Departments:")
			for _, department := range departments.Departments {
				fmt.Printf("%v (%v)\n", department.Code, department.Name)
				fmt.Printf("Type: %v (%v)\n", department.Type.Text, department.Type.Desc)
				fmt.Printf("Work Days: %v\n", department.WorkDays)
				fmt.Printf("Printer: %v (%v)\n", department.Printer.Text, department.Printer.Desc)
				fmt.Printf("Owner: %v (%v)\n", department.Owner.Text, department.Owner.Desc)
				fmt.Println("Served Libraries:")
				for _, library := range department.ServedLibraries.Library {
					fmt.Printf("  %v (%v)\n", library.Text, library.Desc)
				}
				fmt.Println("Operators:")
				for _, operator := range department.Operators.Operator {
					fmt.Printf("  %v (%v)\n", operator.PrimaryID, operator.FullName)
				}
				fmt.Println()
			}
			tables, errs := getCodeTables(requester)
			fmt.Println("Code Tables:")
			for _, table := range tables {
				fmt.Printf("%v (%v)\n", table.Name, table.Description)
				fmt.Printf("Subsystem: %v (%v)\n", table.SubSystem.Text, table.SubSystem.Desc)
				fmt.Printf("Patron Facing: %v\n", table.PatronFacing)
				fmt.Printf("Language: %v (%v)\n", table.Language.Text, table.Language.Desc)
				fmt.Println("Scope:")
				fmt.Printf("  Institution : %v (%v)\n", table.Scope.InstitutionID.Text, table.Scope.InstitutionID.Desc)
				fmt.Printf("  Library : %v (%v)\n", table.Scope.LibraryID.Text, table.Scope.LibraryID.Desc)
				fmt.Println("Rows:")
				for _, row := range table.Rows {
					fmt.Printf("%v (%v) Default: %v Enabled: %v\n", row.Code, row.Description, row.Default, row.Enabled)
				}
				fmt.Println()
			}
			return errs
		},
	}
}

func getLibraries(requester Requester) (libraries Libraries, err error) {
	r, err := http.NewRequest("GET", "/almaws/v1/conf/libraries", nil)
	if err != nil {
		return libraries, err
	}
	body, err := requester(r)
	if err != nil {
		return libraries, err
	}
	err = xml.Unmarshal(body, &libraries)
	if err != nil {
		return libraries, fmt.Errorf("unmarshalling libraries XML failed: %w\n%v", err, string(body))
	}
	return libraries, nil
}

func getDepartments(requester Requester) (departments Departments, err error) {
	r, err := http.NewRequest("GET", "/almaws/v1/conf/departments", nil)
	if err != nil {
		return departments, err
	}
	body, err := requester(r)
	if err != nil {
		return departments, err
	}
	err = xml.Unmarshal(body, &departments)
	if err != nil {
		return departments, fmt.Errorf("unmarshalling departments XML failed: %w\n%v", err, string(body))
	}
	return departments, nil
}

func getCodeTables(requester Requester) (tables []CodeTable, errs []error) {
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
	errorsMux := sync.Mutex{}
	tablesMux := sync.Mutex{}
	jobs := make(chan func())
	wg := sync.WaitGroup{}
	startWorkers(&wg, jobs)
	bar := defaultProgressBar(len(names))
	bar.Describe("Getting code tables")
	for _, name := range names {
		name := name // avoid closure refering to wrong value
		jobs <- func() {
			table, err := getCodeTable(requester, name)
			if err != nil {
				errorsMux.Lock()
				defer errorsMux.Unlock()
				errs = append(errs, err)
			} else {
				tablesMux.Lock()
				defer tablesMux.Unlock()
				tables = append(tables, table)
			}
		}
		// Ignore the possible error returned by the progress bar.
		_ = bar.Add(1)
	}
	close(jobs)
	wg.Wait()
	return tables, errs
}

func getCodeTable(requester Requester, name string) (table CodeTable, err error) {
	r, err := http.NewRequest("GET", "/almaws/v1/conf/code-tables/"+name, nil)
	if err != nil {
		return table, err
	}
	body, err := requester(r)
	if err != nil {
		return table, err
	}
	err = xml.Unmarshal(body, &table)
	if err != nil {
		return table, fmt.Errorf("unmarshalling code table XML failed: %w\n%v", err, string(body))
	}
	return table, nil
}
