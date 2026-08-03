function trim(value) {
	gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
	return value
}

function emit(category_name, item_name, item_value, item_kind) {
	category_name = trim(category_name)
	item_name = trim(item_name)
	item_value = trim(item_value)
	if (category_name == "" || item_name == "") {
		return
	}
	gsub(/\t/, " ", category_name)
	gsub(/\t/, " ", item_name)
	gsub(/\t/, " ", item_value)
	print category_name "\t" item_name "\t" item_value "\t" item_kind
}

function bullet_name(text, position, name) {
	position = index(text, ":")
	if (position > 0) {
		return trim(substr(text, 1, position - 1))
	}
	if (text ~ /^Docker Compose v2 /) {
		return "Docker Compose v2"
	}
	if (text ~ /^Vcpkg /) {
		return "Vcpkg"
	}
	if (match(text, / [vV]?[0-9][0-9A-Za-z.+_-]*/)) {
		name = substr(text, 1, RSTART - 1)
		return trim(name)
	}
	return trim(text)
}

BEGIN {
	category = "Image metadata"
	heading3 = ""
	heading4 = ""
	table_header1 = ""
	table_header2 = ""
	seen_title = 0
}

/^# / {
	seen_title = 1
	next
}

/^### / {
	heading3 = trim(substr($0, 5))
	heading4 = ""
	category = heading3
	table_header1 = ""
	table_header2 = ""
	next
}

/^#### / {
	heading4 = trim(substr($0, 6))
	category = heading3 "/" heading4
	table_header1 = ""
	table_header2 = ""
	next
}

/^## / {
	if ($0 !~ /^## Installed Software/) {
		category = trim(substr($0, 4))
	}
	table_header1 = ""
	table_header2 = ""
	next
}

/^- / {
	text = trim(substr($0, 3))
	name = bullet_name(text)
	value = trim(substr(text, length(name) + 1))
	if (substr(value, 1, 1) == ":") {
		value = trim(substr(value, 2))
	}
	emit(category, name, value, category ~ /Environment variables/ ? "environment" : "software")
	next
}

/^\|/ {
	if (!seen_title) {
		next
	}
	line = $0
	sub(/^\|/, "", line)
	sub(/\|[[:space:]]*$/, "", line)
	column_count = split(line, columns, "|")
	for (column_index = 1; column_index <= column_count; column_index++) {
		columns[column_index] = trim(columns[column_index])
	}
	if (columns[1] == "" || columns[1] ~ /^-+$/ || columns[1] == "Announcements") {
		next
	}
	if (columns[1] == "Name" || columns[1] == "Version" || columns[1] == "Package Name") {
		table_header1 = columns[1]
		table_header2 = columns[2]
		next
	}
	if (table_header1 == "Version" && table_header2 == "Environment Variable") {
		emit(heading3 "/Environment variables", columns[2], columns[1], "environment")
		next
	}
	kind = category ~ /Environment variables/ ? "environment" : "software"
	emit(category, columns[1], columns[2], kind)
	next
}
