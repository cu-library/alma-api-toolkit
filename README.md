# alma-api-toolkit

The Alma API toolkit is a set of tasks which run against the Alma API. It is a replacement for a series of Python scripts which all did similar things:

1. Collect identifiers in an Alma set. 
2. Run a task against all those identifiers. 

Those scripts worked well for smaller sets, but were too unreliable for larger sets.

Sets processed by these tools must be itemized and made public.

## Subcommands

### aat holdings-clean-up-call-numbers

Clean up call numbers in holdings. The following rules are applied:

* Add a space between a number then letter pair.
* Add a space between a number and a period when the period is followed by a letter.
* Remove extra periods from any substring matching space period period (period...).
* Remove any spaces between a period and a number.
* Remove any leading or trailing whitespace.

### aat po-line-update-renewal-date-and-renewal-period

Update the renewal date and renewal period for PO Lines in Alma

The set must be itemized and made public before processing with this tool.

**CAUTION**: There is a known issue with dates and timezone handling. In some cases, the renewal date is set to the day before the one requested. Also, in some other cases, other date fields in the record (like Expected Activation Date) are set to a new value. The new value isn't being set explicitly by this tool. It is the old value of the field minus one day. The bug is confirmed by the API team: https://developers.exlibrisgroup.com/forums/topic/show-and-ask-script-to-update-po-line-records/#post-66403

**CAUTION**: Due to limitations in the Alma API, the notes fields for any PO Line record updated using this tool will all have 'Created On' and 'Updated On' set to today's date, and 'Updated By' will be changed to 'API, Ex Libris'.
