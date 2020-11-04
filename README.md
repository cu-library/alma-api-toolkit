# alma-api-toolkit

The Alma API toolkit is a set of tasks which run against the Alma API. It is a replacement for a series of simple scripts which all did similar things:

1. Collect identifiers in an Alma set.
2. Run a task against all those identifiers.

Those scripts worked well for smaller sets, but were too unreliable and slow for larger sets.

## Advantages

This toolkit improves reliability by retrying failed HTTP calls if required. HTTP requests are performed in parallel so that tasks can be completed more quickly. It also ensures the API call limit is respected.

## Usage Notes

Sets processed by these subcommands must be itemized and made public.

To remove items from a work order, first cancel requests on those items, then scan them in.

```
alma-api-toolkit [FLAGS] subcommand [SUBCOMMAND FLAGS]
  -help
        Print help for this command then exit.
  -key string
        The Alma API key. You can manage your API keys here: https://developers.exlibrisgroup.com/manage/keys/. Required.
  -server string
        The Alma API server to use. (default "api-ca.hosted.exlibrisgroup.com")
  -version
        Print the version then exit.
  Environment variables read when flag is unset:
  ALMAAPITOOLKIT_HELP
  ALMAAPITOOLKIT_KEY
  ALMAAPITOOLKIT_SERVER
  ALMAAPITOOLKIT_VERSION

Subcommands:

items-requests
  View requests on items in the given set.
  -setid string
        The ID of the set we are processing. This flag or setname are required.
  -setname string
        The name of the set we are processing. This flag or setid are required.
  Environment variables read when flag is unset:
  ALMAAPITOOLKIT_ITEMSREQUESTS_SETID
  ALMAAPITOOLKIT_ITEMSREQUESTS_SETNAME

items-cancel-requests
  Cancel item requests of type and/or subtype on items in the given set.
  -setid string
        The ID of the set we are processing. This flag or setname are required.
  -setname string
        The name of the set we are processing. This flag or setid are required.
  -subtype string
        The request subtype to cancel.
  -type string
        The request type to cancel. ex: WORK_ORDER
  Environment variables read when flag is unset:
  ALMAAPITOOLKIT_ITEMSCANCELREQUESTS_SETID
  ALMAAPITOOLKIT_ITEMSCANCELREQUESTS_SETNAME
  ALMAAPITOOLKIT_ITEMSCANCELREQUESTS_SUBTYPE
  ALMAAPITOOLKIT_ITEMSCANCELREQUESTS_TYPE

items-scan-in
  Scan items in.
  -circdesk string
        The circ desk code. The possible values are not available through the API, see https://developers.exlibrisgroup.com/alma/apis/docs/xsd/rest_item_loan.xsd/?tags=GET. (default "DEFAULT_CIRC_DESK")
  -library string
        The library code. Use the conf-libaries-departments-code-tables subcommand to see the possible values.
  -setid string
        The ID of the set we are processing. This flag or setname are required.
  -setname string
        The name of the set we are processing. This flag or setid are required.
  Environment variables read when flag is unset:
  ALMAAPITOOLKIT_ITEMSSCANIN_CIRCDESK
  ALMAAPITOOLKIT_ITEMSSCANIN_LIBRARY
  ALMAAPITOOLKIT_ITEMSSCANIN_SETID
  ALMAAPITOOLKIT_ITEMSSCANIN_SETNAME

conf-libaries-departments-code-tables
  Print the output of the library and departments endpoints, and the known code tables.
  The list of known code tables comes from
  https://developers.exlibrisgroup.com/blog/almas-code-tables-api-list-of-code-tables/
  This command is meant to help run other subcommands which sometimes need a particular code
  from a code table or the code for a library or department.

holdings-clean-up-call-numbers
  Clean up the call numbers on a set of holdings records.
  A CSV report of the changes made is printed to stdout.

  The following rules are run on the call numbers:
  - Add a space between a number then a letter.
  - Add a space between a number and a period when the period is followed by a letter.
  - Remove the extra periods from any substring matching space period period...
  - Remove any spaces between a period and a number.
  - Remove any leading or trailing whitespace.

  -dryrun
        Do not perform any updates. Report on what changes would have been made.
  -setid string
        The ID of the set we are processing. This flag or setname are required.
  -setname string
        The name of the set we are processing. This flag or setid are required.
  Environment variables read when flag is unset:
  ALMAAPITOOLKIT_HOLDINGSCLEANUPCALLNUMBERS_DRYRUN
  ALMAAPITOOLKIT_HOLDINGSCLEANUPCALLNUMBERS_SETID
  ALMAAPITOOLKIT_HOLDINGSCLEANUPCALLNUMBERS_SETNAME
```

## Subcommand Notes

### po-line-update-renewal-date-and-renewal-period (not done)

Update the renewal date and renewal period for PO Lines in Alma

The set must be itemized and made public before processing with this tool.

**CAUTION**: There is a known issue with dates and timezone handling. In some cases, the renewal date is set to the day before the one requested. Also, in some other cases, other date fields in the record (like Expected Activation Date) are set to a new value. The new value isn't being set explicitly by this tool. It is the old value of the field minus one day. The bug is confirmed by the API team: https://developers.exlibrisgroup.com/forums/topic/show-and-ask-script-to-update-po-line-records/#post-66403

**CAUTION**: Due to limitations in the Alma API, the notes fields for any PO Line record updated using this tool will all have 'Created On' and 'Updated On' set to today's date, and 'Updated By' will be changed to 'API, Ex Libris'.

### holdings-clean-up-call-numbers

This subcommand outputs a CSV report of what holdings records had their call numbers updated. You can redirect the output to a file using your shell.

```
Before                After
BR115.C5L43           BR115 .C5 L43
BS410.V452 V. 31      BS410 .V452 V.31
```

### items-requests

View user requests on items in a given set. The item request type and subtype can then be used to cancel requests using the `items-cancel-requests` subcommand. Only the type and subtype are printed, as that is the information that is needed to cancel the requests.
