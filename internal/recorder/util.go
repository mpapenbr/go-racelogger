package recorder

//nolint:whitespace // editor/linter issue
func computeNameAndDescription(cliNames, cliDescr []string, idx int) (
	name, description string,
) {
	name = ""
	description = ""
	if len(cliNames) > 0 {
		nameIdx := idx
		if nameIdx >= len(cliNames) {
			nameIdx = len(cliNames) - 1 // use last name if index is out of range
		}
		name = cliNames[nameIdx]
	}

	if len(cliDescr) > 0 {
		descrIdx := idx
		if descrIdx >= len(cliDescr) {
			descrIdx = len(cliDescr) - 1 // use last description if index is out of range
		}
		description = cliDescr[descrIdx]
	}
	return name, description
}
