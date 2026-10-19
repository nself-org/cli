# Feature: ɳFamily

ɳFamily is a planned private social media platform for families, built on ɳSelf. It provides a self-hosted space for family members to share photos, updates, and conversations without any data leaving your own server.

**Status:** Planned (not in current development scope)
**Repo:** TBD
**Relationship to Flock:** Independent. ɳFamily is an ɳSelf product. Flock is a Unity (unyeco) product. They share no code or infrastructure. The privacy boundary between the two projects is intentional.

---

## Concept

Most families share photos and updates across a mix of iMessage, WhatsApp, Instagram, and Google Photos. None of these are private. None of them give you ownership of your data. ɳFamily is a self-hosted alternative where:

- **You own the server.** All photos, posts, and messages stay on hardware you control.
- **No ads, no algorithms.** The feed is chronological. No engagement optimization.
- **Invitation only.** Family members join via invite codes. No public profiles, no discoverability.
- **Multi-generational.** Simple enough for grandparents. Parental controls for kids.
- **One subscription.** The ɳFamily plugin bundle runs on your existing ɳSelf server. No per-user pricing.

---

## Planned Features

### Photo Sharing
- Upload photos and albums from any device
- Automatic organization by date and location
- Face detection and grouping (local processing, no cloud)
- Shared albums with comments and reactions
- Original quality preservation (no compression)

### Activity Feed
- Chronological timeline of family activity
- Posts with text, photos, and location
- Reactions and threaded comments
- Birthday and anniversary reminders

### Messaging
- Real-time family group chat
- Direct messages between family members
- Photo and video sharing in chat
- Voice messages

### Content Moderation
- Parental controls per family member
- Content filtering for younger members
- Activity reports for parents
- Screen time awareness (not enforcement, that is the device's job)

### Privacy
- All data stored on your server. Zero cloud dependencies.
- No analytics, no tracking, no telemetry
- End-to-end encryption for direct messages (planned)
- Granular sharing controls (who sees what)

---

## ɳFamily Plugin Bundle

The ɳFamily bundle uses these ɳSelf Licensed plugins:

| Plugin | Purpose |
|--------|---------|
| `social` | Activity feed, posts, reactions, follows |
| `photos` | Photo upload, organization, face detection |
| `activity-feed` | Timeline aggregation and display |
| `moderation` | Content filtering and parental controls |
| `realtime` | WebSocket connections for live updates |
| `cms` | Content management for family pages/blogs |
| `chat` | Real-time messaging |

These plugins are shared with other ɳSelf products. The ɳFamily client app combines them into a family-specific workflow.

---

## Architecture

ɳFamily follows the same pattern as other ɳSelf reference apps:

1. **Flutter client app** (iOS, Android, Web) communicates with the ɳSelf backend via Hasura GraphQL.
2. **ɳSelf backend** runs the ɳFamily plugin bundle. Plugins handle photo processing, feed generation, real-time messaging, and moderation.
3. **MinIO** stores photos and media at original quality.
4. **Auth** handles family member accounts, invitations, and role-based access.

No dedicated server is required. ɳFamily runs alongside your other ɳSelf apps on the same server.

---

## Pricing

ɳFamily will be available as a plugin bundle at $0.99/mo or $9.99/yr. The ɳSelf+ subscription ($49/yr) includes ɳFamily along with all other plugin bundles.

---

## Relationship to Flock

Flock (unyeco/flock) is a separate family app built on Unity infrastructure. ɳFamily and Flock are completely independent:

| | ɳFamily | Flock |
|---|---------|-------|
| **Organization** | ɳSelf (nself-org) | Unity (unyeco) |
| **Backend** | ɳSelf CLI stack | Unity shared backend |
| **Hosting** | Self-hosted by user | Unity servers |
| **Data ownership** | User owns everything | Unity-managed |
| **Target** | Privacy-first families | Social families |
| **Status** | Planned | Active |

The separation is by design. ɳFamily serves users who want complete data ownership. Flock serves users who prefer a managed workflow.

---

## Related Pages

- [[Feature-Plugins]] -- plugin system overview
- [[Plugin-Licensing]] -- bundle pricing

---

← [[Features]] | [[_Sidebar]]
