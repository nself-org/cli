-- ============================================================================
-- Simple Blog Database Schema
-- ============================================================================
-- This schema creates tables for a basic blog application with posts,
-- comments, and user relationships.
--
-- Author: nself
-- Version: 0.9.8
-- ============================================================================

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ============================================================================
-- POSTS TABLE
-- ============================================================================
-- Stores blog posts with title, content, author, and publication status

CREATE TABLE posts (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  title TEXT NOT NULL CHECK (char_length(title) >= 1 AND char_length(title) <= 200),
  slug TEXT UNIQUE NOT NULL CHECK (char_length(slug) >= 1 AND char_length(slug) <= 250),
  content TEXT NOT NULL CHECK (char_length(content) >= 1),
  excerpt TEXT CHECK (excerpt IS NULL OR char_length(excerpt) <= 500),
  author_id UUID NOT NULL,
  published BOOLEAN DEFAULT false,
  published_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

  -- Foreign key to auth.users (managed by nHost Auth)
  CONSTRAINT fk_author
    FOREIGN KEY (author_id)
    REFERENCES auth.users(id)
    ON DELETE CASCADE
);

-- Comments on table
COMMENT ON TABLE posts IS 'Blog posts with title, content, and metadata';
COMMENT ON COLUMN posts.title IS 'Post title (1-200 characters)';
COMMENT ON COLUMN posts.slug IS 'URL-friendly slug (auto-generated from title)';
COMMENT ON COLUMN posts.content IS 'Full post content (markdown supported)';
COMMENT ON COLUMN posts.excerpt IS 'Short excerpt for previews (max 500 chars)';
COMMENT ON COLUMN posts.published IS 'Whether post is published or draft';
COMMENT ON COLUMN posts.published_at IS 'Publication timestamp';

-- ============================================================================
-- COMMENTS TABLE
-- ============================================================================
-- Stores comments on blog posts

CREATE TABLE comments (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  post_id UUID NOT NULL,
  author_id UUID NOT NULL,
  content TEXT NOT NULL CHECK (char_length(content) >= 1 AND char_length(content) <= 2000),
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

  -- Foreign keys
  CONSTRAINT fk_post
    FOREIGN KEY (post_id)
    REFERENCES posts(id)
    ON DELETE CASCADE,

  CONSTRAINT fk_comment_author
    FOREIGN KEY (author_id)
    REFERENCES auth.users(id)
    ON DELETE CASCADE
);

-- Comments on table
COMMENT ON TABLE comments IS 'Comments on blog posts';
COMMENT ON COLUMN comments.content IS 'Comment text (max 2000 characters)';

-- ============================================================================
-- INDEXES
-- ============================================================================
-- Performance indexes for common queries

-- Posts indexes
CREATE INDEX idx_posts_author ON posts(author_id);
CREATE INDEX idx_posts_published ON posts(published, published_at DESC) WHERE published = true;
CREATE INDEX idx_posts_slug ON posts(slug);
CREATE INDEX idx_posts_created ON posts(created_at DESC);

-- Comments indexes
CREATE INDEX idx_comments_post ON comments(post_id, created_at);
CREATE INDEX idx_comments_author ON comments(author_id);

-- ============================================================================
-- TRIGGERS
-- ============================================================================

-- Updated_at trigger function
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION update_updated_at_column() IS 'Auto-update updated_at column on row changes';

-- Apply trigger to posts
CREATE TRIGGER update_posts_updated_at
  BEFORE UPDATE ON posts
  FOR EACH ROW
  EXECUTE FUNCTION update_updated_at_column();

-- Apply trigger to comments
CREATE TRIGGER update_comments_updated_at
  BEFORE UPDATE ON comments
  FOR EACH ROW
  EXECUTE FUNCTION update_updated_at_column();

-- ============================================================================
-- SLUG GENERATION
-- ============================================================================

-- Function to generate URL-friendly slug from title
CREATE OR REPLACE FUNCTION generate_slug(title TEXT)
RETURNS TEXT AS $$
DECLARE
  slug TEXT;
BEGIN
  -- Convert to lowercase
  slug := lower(title);

  -- Remove special characters (keep alphanumeric, spaces, and hyphens)
  slug := regexp_replace(slug, '[^a-z0-9\s-]', '', 'g');

  -- Replace spaces with hyphens
  slug := regexp_replace(slug, '\s+', '-', 'g');

  -- Remove multiple consecutive hyphens
  slug := regexp_replace(slug, '-+', '-', 'g');

  -- Trim hyphens from start and end
  slug := trim(both '-' from slug);

  -- Limit length
  IF char_length(slug) > 200 THEN
    slug := substring(slug, 1, 200);
    -- Remove trailing partial word
    slug := regexp_replace(slug, '-[^-]*$', '');
  END IF;

  RETURN slug;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

COMMENT ON FUNCTION generate_slug(TEXT) IS 'Generate URL-friendly slug from title';

-- Auto-generate slug on insert if not provided
CREATE OR REPLACE FUNCTION auto_generate_slug()
RETURNS TRIGGER AS $$
DECLARE
  base_slug TEXT;
  final_slug TEXT;
  counter INTEGER := 1;
BEGIN
  -- Only generate if slug is null or empty
  IF NEW.slug IS NULL OR NEW.slug = '' THEN
    base_slug := generate_slug(NEW.title);
    final_slug := base_slug;

    -- Check for uniqueness and append counter if needed
    WHILE EXISTS (SELECT 1 FROM posts WHERE slug = final_slug AND id != COALESCE(NEW.id, uuid_nil())) LOOP
      final_slug := base_slug || '-' || counter;
      counter := counter + 1;
    END LOOP;

    NEW.slug := final_slug;
  END IF;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION auto_generate_slug() IS 'Auto-generate unique slug for posts';

CREATE TRIGGER posts_auto_slug
  BEFORE INSERT OR UPDATE ON posts
  FOR EACH ROW
  EXECUTE FUNCTION auto_generate_slug();

-- ============================================================================
-- PUBLISH TRIGGER
-- ============================================================================

-- Auto-set published_at when post is published
CREATE OR REPLACE FUNCTION set_published_at()
RETURNS TRIGGER AS $$
BEGIN
  -- Set published_at when published changes from false to true
  IF NEW.published = true AND (OLD.published = false OR OLD.published IS NULL) THEN
    NEW.published_at := NOW();
  END IF;

  -- Clear published_at when unpublished
  IF NEW.published = false AND OLD.published = true THEN
    NEW.published_at := NULL;
  END IF;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION set_published_at() IS 'Auto-set published_at timestamp when post is published';

CREATE TRIGGER posts_set_published_at
  BEFORE INSERT OR UPDATE ON posts
  FOR EACH ROW
  EXECUTE FUNCTION set_published_at();

-- ============================================================================
-- VIEWS
-- ============================================================================

-- View for published posts with author info
CREATE OR REPLACE VIEW published_posts AS
SELECT
  p.id,
  p.title,
  p.slug,
  p.excerpt,
  p.content,
  p.published_at,
  p.created_at,
  p.updated_at,
  p.author_id,
  u.display_name AS author_name,
  u.avatar_url AS author_avatar,
  (SELECT COUNT(*) FROM comments c WHERE c.post_id = p.id) AS comment_count
FROM posts p
JOIN auth.users u ON p.author_id = u.id
WHERE p.published = true
ORDER BY p.published_at DESC;

COMMENT ON VIEW published_posts IS 'Published posts with author info and comment counts';

-- ============================================================================
-- FUNCTIONS
-- ============================================================================

-- Function to get post stats
CREATE OR REPLACE FUNCTION get_post_stats(post_uuid UUID)
RETURNS TABLE (
  comment_count BIGINT,
  first_comment_date TIMESTAMP WITH TIME ZONE,
  last_comment_date TIMESTAMP WITH TIME ZONE
) AS $$
BEGIN
  RETURN QUERY
  SELECT
    COUNT(*)::BIGINT,
    MIN(created_at),
    MAX(created_at)
  FROM comments
  WHERE post_id = post_uuid;
END;
$$ LANGUAGE plpgsql STABLE;

COMMENT ON FUNCTION get_post_stats(UUID) IS 'Get statistics for a post (comments, dates)';

-- ============================================================================
-- INITIAL DATA (Optional)
-- ============================================================================

-- Note: Actual user creation is handled by nHost Auth service
-- To add sample posts, run the seed data script after creating a user

-- ============================================================================
-- GRANTS
-- ============================================================================

-- Grant permissions to authenticated users (handled by Hasura permissions)
-- See Hasura Console for role-based access control configuration

-- ============================================================================
-- END OF SCHEMA
-- ============================================================================

-- Verify installation
DO $$
BEGIN
  RAISE NOTICE 'Blog schema installed successfully!';
  RAISE NOTICE 'Tables created: posts, comments';
  RAISE NOTICE 'Views created: published_posts';
  RAISE NOTICE 'Functions created: generate_slug, get_post_stats';
  RAISE NOTICE '';
  RAISE NOTICE 'Next steps:';
  RAISE NOTICE '1. Create a user via Auth service';
  RAISE NOTICE '2. Run database/seeds/sample-data.sql';
  RAISE NOTICE '3. Configure Hasura metadata';
  RAISE NOTICE '4. Start building!';
END $$;
