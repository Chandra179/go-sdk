CREATE TABLE IF NOT EXISTS users (
    -- UUID v7 (or default PostgreSQL UUID v4)
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- Email is often synced from OIDC, but can change.
    -- Treat it as mutable data, not an ID.
    email VARCHAR(255) NOT NULL,
    
    full_name VARCHAR(100),
    
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS federated_identities (
    -- The 'iss' (Issuer) claim from the ID Token.
    -- Example: 'https://accounts.google.com' or 'https://github.com'
    provider VARCHAR(255) NOT NULL,
    
    -- The 'sub' (Subject) claim from the ID Token.
    -- Example: '10769150350006150715113' (Google) or '583231' (GitHub)
    -- WARNING: These are strings, not always integers.
    subject_id VARCHAR(255) NOT NULL,
    
    -- The link to your internal user
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    
    -- Metadata (Optional but useful for debugging/audits)
    last_login_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- COMPOSITE PRIMARY KEY
    -- A user is unique based on WHO they logged in with (provider)
    -- and THEIR ID on that platform (subject_id).
    PRIMARY KEY (provider, subject_id)
);

-- Create indexes
-- Fast lookup to find all linked accounts for a specific internal user
CREATE INDEX IF NOT EXISTS idx_federated_identities_user_id ON federated_identities(user_id);

-- Optional: Index for email lookup (if you need to find users by email)
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);