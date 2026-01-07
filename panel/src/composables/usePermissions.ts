import { computed, type ComputedRef } from 'vue';
import type { Role } from '@/types';

/**
 * Role hierarchy levels for permission checks.
 * Higher number = more permissions.
 */
const ROLE_LEVELS: Record<string, number> = {
    // Standard role names
    VIEWER: 1,
    USER: 2,
    ADMIN: 3,
    OWNER: 4,
    // Legacy role names (for backwards compatibility)
    READ_ONLY: 1,
    READ_WRITE: 2,
};

export interface PermissionFlags {
    /** Can view workspace data (VIEWER+) */
    canView: ComputedRef<boolean>;
    /** Can create/edit agents and probes (USER+) */
    canEdit: ComputedRef<boolean>;
    /** Can manage members, delete resources (ADMIN+) */
    canManage: ComputedRef<boolean>;
    /** Full control including workspace deletion (OWNER only) */
    canOwn: ComputedRef<boolean>;

    // Granular permissions
    canCreateAgent: ComputedRef<boolean>;
    canEditAgent: ComputedRef<boolean>;
    canDeleteAgent: ComputedRef<boolean>;
    canCreateProbe: ComputedRef<boolean>;
    canEditProbe: ComputedRef<boolean>;
    canDeleteProbe: ComputedRef<boolean>;
    canInviteMembers: ComputedRef<boolean>;
    canEditMembers: ComputedRef<boolean>;
    canRemoveMembers: ComputedRef<boolean>;
    canEditWorkspace: ComputedRef<boolean>;
    canDeleteWorkspace: ComputedRef<boolean>;
    canTransferOwnership: ComputedRef<boolean>;

    /** Raw role level for custom checks */
    roleLevel: ComputedRef<number>;
}

/**
 * Composable for checking user permissions based on their workspace role.
 * 
 * @example
 * ```vue
 * <script setup>
 * import { usePermissions } from '@/composables/usePermissions';
 * 
 * const { canEdit, canManage } = usePermissions(userRole);
 * </script>
 * 
 * <template>
 *   <button v-if="canEdit" @click="edit">Edit</button>
 *   <button v-if="canManage" @click="delete">Delete</button>
 * </template>
 * ```
 */
export function usePermissions(role: Role | string | undefined | null): PermissionFlags {
    const roleLevel = computed(() => {
        if (!role) return 0;
        return ROLE_LEVELS[role.toUpperCase()] || 0;
    });

    // Base permission levels
    const canView = computed(() => roleLevel.value >= 1);
    const canEdit = computed(() => roleLevel.value >= 2);
    const canManage = computed(() => roleLevel.value >= 3);
    const canOwn = computed(() => roleLevel.value >= 4);

    return {
        canView,
        canEdit,
        canManage,
        canOwn,
        roleLevel,

        // Agent permissions
        canCreateAgent: canEdit,
        canEditAgent: canEdit,
        canDeleteAgent: canManage,

        // Probe permissions
        canCreateProbe: canEdit,
        canEditProbe: canEdit,
        canDeleteProbe: canManage,

        // Member permissions
        canInviteMembers: canManage,
        canEditMembers: canManage,
        canRemoveMembers: canManage,

        // Workspace permissions
        canEditWorkspace: canManage,
        canDeleteWorkspace: canOwn,
        canTransferOwnership: canOwn,
    };
}

/**
 * Check if a role meets a minimum required level.
 * 
 * @example
 * ```typescript
 * if (hasMinimumRole(userRole, 'ADMIN')) {
 *   // User is ADMIN or OWNER
 * }
 * ```
 */
export function hasMinimumRole(userRole: string | undefined, minRole: string): boolean {
    if (!userRole) return false;
    const userLevel = ROLE_LEVELS[userRole.toUpperCase()] || 0;
    const minLevel = ROLE_LEVELS[minRole.toUpperCase()] || 0;
    return userLevel >= minLevel;
}

/**
 * Get human-readable role name.
 */
export function getRoleDisplayName(role: string): string {
    const names: Record<string, string> = {
        OWNER: 'Owner',
        ADMIN: 'Admin',
        USER: 'User',
        VIEWER: 'Viewer',
        READ_WRITE: 'User',
        READ_ONLY: 'Viewer',
    };
    return names[role?.toUpperCase()] || role || 'Unknown';
}

/**
 * Get role badge color class.
 */
export function getRoleBadgeClass(role: string): string {
    const classes: Record<string, string> = {
        OWNER: 'bg-danger',
        ADMIN: 'bg-warning text-dark',
        USER: 'bg-success',
        VIEWER: 'bg-secondary',
        READ_WRITE: 'bg-success',
        READ_ONLY: 'bg-secondary',
    };
    return classes[role?.toUpperCase()] || 'bg-secondary';
}
