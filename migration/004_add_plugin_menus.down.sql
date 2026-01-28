-- Rollback: Remove plugin_name column from menus table

-- Drop indexes
DROP INDEX `idx_menus_plugin_active` ON `menus`;
DROP INDEX `idx_menus_plugin_name` ON `menus`;

-- Remove plugin_name column
ALTER TABLE `menus` DROP COLUMN `plugin_name`;
