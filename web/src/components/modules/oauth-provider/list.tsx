
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table"
import { Button } from "@/components/ui/button"
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { MoreHorizontal, Plus, RefreshCw, Pencil, Trash } from "lucide-react"
import { useOAuthProviderList, useDeleteOAuthProvider, useRefreshOAuthProvider, OAuthProvider } from "@/api/endpoints/oauthProvider"
import { formatTime } from "@/lib/utils"
import { Badge } from "@/components/ui/badge"
import { useState } from "react"
import { CreateEditOAuthProviderModal } from "./create-modal"
import { useTranslations } from "next-intl"

export function OAuthProviderList() {
    const { data: providers, isLoading } = useOAuthProviderList()
    const deleteMutation = useDeleteOAuthProvider()
    const refreshMutation = useRefreshOAuthProvider()
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
                            <TableHead>{t("status")}</TableHead>
                            <TableHead>{t("lastRefresh")}</TableHead>
                            <TableHead>{t("createdAt")}</TableHead>
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
                                <TableCell>
                                    <Badge variant={provider.status === 1 ? "default" : "secondary"}>
                                        {provider.status === 1 ? t("active") : t("inactive")}
                                    </Badge>
                                </TableCell>
                                <TableCell>{formatTime(provider.last_refresh_at)}</TableCell>
                                <TableCell>{formatTime(provider.created_at)}</TableCell>
                                <TableCell className="text-right">
                                    <DropdownMenu>
                                        <DropdownMenuTrigger asChild>
                                            <Button variant="ghost" className="h-8 w-8 p-0">
                                                <span className="sr-only">Open menu</span>
                                                <MoreHorizontal className="h-4 w-4" />
                                            </Button>
                                        </DropdownMenuTrigger>
                                        <DropdownMenuContent align="end">
                                            <DropdownMenuItem onClick={() => handleRefresh(provider.id)}>
                                                <RefreshCw className="mr-2 h-4 w-4" />
                                                {t("refresh")}
                                            </DropdownMenuItem>
                                            <DropdownMenuItem onClick={() => handleEdit(provider)}>
                                                <Pencil className="mr-2 h-4 w-4" />
                                                {t("edit")}
                                            </DropdownMenuItem>
                                            <DropdownMenuItem
                                                onClick={() => handleDelete(provider.id)}
                                                className="text-red-600 focus:text-red-600"
                                            >
                                                <Trash className="mr-2 h-4 w-4" />
                                                {t("delete")}
                                            </DropdownMenuItem>
                                        </DropdownMenuContent>
                                    </DropdownMenu>
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
