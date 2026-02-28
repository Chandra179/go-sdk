CREATE TABLE IF NOT EXISTS users (
    -- UUID v7 (or default PostgreSQL UUID v4)
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- OAuth provider (e.g., 'google', 'github')
    provider VARCHAR(255) NOT NULL,
    
    -- The 'sub' (Subject) claim from the ID Token
    -- Example: '10769150350006150715113' (Google) or '583231' (GitHub)
    subject_id VARCHAR(255) NOT NULL,
    
    -- Email is often synced from OIDC, but can change
    email VARCHAR(255) NOT NULL,
    
    full_name VARCHAR(100),
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Unique constraint on provider + subject_id
    UNIQUE(provider, subject_id)
);

-- Optional: Index for email lookup
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);