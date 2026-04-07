-- Check migration status
SELECT version, dirty FROM schema_migrations ORDER BY version;

-- Check if properties table exists
SELECT 
    CASE WHEN EXISTS (
        SELECT 1 FROM information_schema.tables 
        WHERE table_schema = 'public' AND table_name = 'properties'
    ) 
    THEN 'EXISTS' 
    ELSE 'NOT EXISTS' 
END AS table_status;
