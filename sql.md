CREATE TABLE `groups` (
    `id` integer PRIMARY KEY AUTOINCREMENT,
    `name` text NOT NULL,
    `mode` integer NOT NULL,
    `match_regex` text,
    `first_token_time_out` integer,
    `session_keep_time` integer,
    CONSTRAINT `uni_groups_name` UNIQUE (`name`)
);
CREATE TABLE `group_items` (
    `id` integer PRIMARY KEY AUTOINCREMENT,
    `group_id` integer NOT NULL,
    `channel_id` integer NOT NULL,
    `model_name` text NOT NULL,
    `priority` integer,
    `weight` integer,
    CONSTRAINT `fk_groups_items` FOREIGN KEY (`group_id`) REFERENCES `groups` (`id`)
);

CREATE UNIQUE INDEX `idx_group_channel_model` ON `group_items` (`group_id`, `channel_id`, `model_name`);