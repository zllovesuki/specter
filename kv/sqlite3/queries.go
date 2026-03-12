package sqlite3

// ---------------------------------------------------------------------------
// Centralized SQL query definitions for kv/sqlite3.
//
// Fixed-shape queries are grouped by domain and referenced by the prepared
// statement registry (see statements.go) as well as direct call sites.
// Dynamic queries (e.g. RemoveKeys with variable IN clauses) stay in their
// respective call sites but use the table/column names defined here.
// ---------------------------------------------------------------------------

// -- simple_entries ---------------------------------------------------------

const (
	querySimplePut = "INSERT INTO `simple_entries` (`key`, `value`) VALUES (?, ?) ON CONFLICT(`key`) DO UPDATE SET `value` = excluded.`value`"
	querySimpleGet = "SELECT `value` FROM `simple_entries` WHERE `key` = ?"
	querySimpleDel = "DELETE FROM `simple_entries` WHERE `key` = ?"
)

// -- prefix_entries ---------------------------------------------------------

const (
	queryPrefixAppend   = "INSERT INTO `prefix_entries` (`prefix`, `child`) VALUES (?, ?) ON CONFLICT DO NOTHING"
	queryPrefixContains = "SELECT 1 FROM `prefix_entries` WHERE `prefix` = ? AND `child` = ? LIMIT 1"
	queryPrefixList     = "SELECT `child` FROM `prefix_entries` WHERE `prefix` = ?"
	queryPrefixRemove   = "DELETE FROM `prefix_entries` WHERE `prefix` = ? AND `child` = ?"
	queryPrefixCount    = "SELECT COUNT(*) FROM `prefix_entries` WHERE `prefix` = ?"
)

// -- lease_entries ----------------------------------------------------------

const (
	queryLeaseAcquire = "INSERT INTO `lease_entries` (`owner`, `token`) VALUES (?, ?) " +
		"ON CONFLICT(`owner`) DO UPDATE SET `token` = excluded.`token` " +
		"WHERE `token` <= ?"
	queryLeaseRenew   = "UPDATE `lease_entries` SET `token` = ? WHERE `owner` = ? AND `token` = ? AND `token` > ?"
	queryLeaseRelease = "DELETE FROM `lease_entries` WHERE `owner` = ? AND `token` = ?"
	queryLeaseGet     = "SELECT `token` FROM `lease_entries` WHERE `owner` = ?"
	queryLeaseImport  = "INSERT INTO `lease_entries` (`owner`, `token`) VALUES (?, ?) ON CONFLICT(`owner`) DO UPDATE SET `token` = excluded.`token`"
)

// -- key_trackers -----------------------------------------------------------

const (
	queryTrackerLookup = "SELECT `hash`, `flags` FROM `key_trackers` WHERE `key` = ?"
	queryTrackerInsert = "INSERT INTO `key_trackers` (`key`, `hash`, `flags`) VALUES (?, ?, ?)"
	queryTrackerUpdate = "UPDATE `key_trackers` SET `flags` = ? WHERE `key` = ?"
	queryTrackerDelete = "DELETE FROM `key_trackers` WHERE `key` = ?"
)

// -- provider (list / range) ------------------------------------------------

const (
	queryListKeys      = "SELECT `key`, `flags` FROM `key_trackers`"
	queryRangeKeysNorm = "SELECT `key` FROM `key_trackers` WHERE (`hash` > ? AND `hash` < ?) OR `hash` = ? ORDER BY `hash` ASC"
	queryRangeKeysWrap = "SELECT `key` FROM `key_trackers` WHERE `hash` > ? OR `hash` < ? OR `hash` = ? ORDER BY `hash` ASC"
)
