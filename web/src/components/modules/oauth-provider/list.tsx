
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table"
import { Button } from "@/components/ui/button"
import { Switch } from "@/components/ui/switch"
import { Plus, RefreshCw, Pencil, Trash } from "lucide-react"
import { useOAuthProviderList, useDeleteOAuthProvider, useRefreshOAuthProvider, useUpdateOAuthProvider, OAuthProvider } from "@/api/endpoints/oauthProvider"
import { formatDateTime } from "@/lib/utils"
import { Badge } from "@/components/ui/badge"
import { useState } from "react"
import { CreateEditOAuthProviderModal } from "./create-modal"
import { useTranslations } from "next-intl"

export function OAuthProviderList() {
    const { data: providers, isLoading } = useOAuthProviderList()
    const deleteMutation = useDeleteOAuthProvider()
    const refreshMutation = useRefreshOAuthProvider()
    const updateMutation = useUpdateOAuthProvider()
    const [isCreateOpen, setIsCreateOpen] = useState(false)
    const [editingProvider, setEditingProvider] = useState<OAuthProvider | null>(null)
    const t = useTranslations("oauthProvider")

    if (isLoading) {
        return <div>Loading...</div>
    }

    const handleDelete = (id: number) => {
        if (confirm(t("confirmDelete"))) {
            deleteMutation.mutate(id)
        }
    }

    const handleRefresh = (id: number) => {
        refreshMutation.mutate(id)
    }

    const handleEdit = (provider: OAuthProvider) => {
        setEditingProvider(provider)
        setIsCreateOpen(true)
    }

    const handleToggleStatus = (provider: OAuthProvider) => {
        updateMutation.mutate({
            id: provider.id,
            status: provider.status === 1 ? 0 : 1,
        })
    }

    return (
        <div className="space-y-4">
            <div className="flex justify-between items-center">
                <h2 className="text-2xl font-bold tracking-tight">{t("title")}</h2>
                <Button onClick={() => { setEditingProvider(null); setIsCreateOpen(true) }}>
                    <Plus className="mr-2 h-4 w-4" />
                    {t("create")}
                </Button>
            </div>

            <div className="rounded-md border">
                <Table>
                    <TableHeader>
                        <TableRow>
                            <TableHead>ID</TableHead>
                            <TableHead>{t("name")}</TableHead>
                            <TableHead>{t("type")}</TableHead>
                            <TableHead>{t("lastRefresh")}</TableHead>
                            <TableHead>{t("createdAt")}</TableHead>
                            <TableHead>{t("enabled")}</TableHead>
                            <TableHead className="text-right">{t("actions")}</TableHead>
                        </TableRow>
                    </TableHeader>
                    <TableBody>
                        {providers?.map((provider) => (
                            <TableRow key={provider.id}>
                                <TableCell>{provider.id}</TableCell>
                                <TableCell className="font-medium">{provider.name}</TableCell>
                                <TableCell>
                                    <Badge variant="outline">{provider.provider_type}</Badge>
                                </TableCell>
                                <TableCell>{formatDateTime(provider.last_refresh_at)}</TableCell>
                                <TableCell>{formatDateTime(provider.created_at)}</TableCell>
                                <TableCell>
                                    <Switch
                                        checked={provider.status === 1}
                                        onCheckedChange={() => handleToggleStatus(provider)}
                                    />
                                </TableCell>
                                <TableCell className="text-right">
                                    <div className="flex items-center justify-end gap-1">
                                        <Button
                                            variant="ghost"
                                            size="sm"
                                            onClick={() => handleRefresh(provider.id)}
                                            disabled={refreshMutation.isPending}
                                        >
                                            <RefreshCw className={`h-4 w-4 ${refreshMutation.isPending ? 'animate-spin' : ''}`} />
                                        </Button>
                                        <Button
                                            variant="ghost"
                                            size="sm"
                                            onClick={() => handleEdit(provider)}
                                        >
                                            <Pencil className="h-4 w-4" />
                                        </Button>
                                        <Button
                                            variant="ghost"
                                            size="sm"
                                            onClick={() => handleDelete(provider.id)}
                                            className="text-destructive hover:text-destructive"
                                        >
                                            <Trash className="h-4 w-4" />
                                        </Button>
                                    </div>
                                </TableCell>
                            </TableRow>
                        ))}
                        {(!providers || providers.length === 0) && (
                            <TableRow>
                                <TableCell colSpan={7} className="text-center h-24 text-muted-foreground">
                                    {t("noData")}
                                </TableCell>
                            </TableRow>
                        )}
                    </TableBody>
                </Table>
            </div>

            <CreateEditOAuthProviderModal
                open={isCreateOpen}
                onOpenChange={setIsCreateOpen}
                provider={editingProvider}
            />
        </div>
    )
}
