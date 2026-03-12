CREATE TABLE IF NOT EXISTS `key_trackers` (
    `key` blob,
    `hash` integer,
    `flags` integer,
    PRIMARY KEY (`key`)
);

CREATE INDEX IF NOT EXISTS `idx_hash` ON `key_trackers` (`hash` ASC);

CREATE TABLE IF NOT EXISTS `simple_entries` (
    `key` blob,
    `value` blob,
    PRIMARY KEY (`key`)
);

CREATE TABLE IF NOT EXISTS `prefix_entries` (
    `prefix` blob,
    `child` blob,
    PRIMARY KEY (`prefix`, `child`)
);

CREATE TABLE IF NOT EXISTS `lease_entries` (
    `owner` blob,
    `token` integer,
    PRIMARY KEY (`owner`)
);
