import { Button } from "@/components/ui/button"
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
    DialogFooter,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Badge } from "@/components/ui/badge"
import { Switch } from "@/components/ui/switch"
import { useForm } from "react-hook-form"
import { useEffect, useState, useRef } from "react"
import { useCreateOAuthProvider, useUpdateOAuthProvider, useFetchOAuthModel, useGetOAuthAuthURL, useHandleOAuthCallback, useOAuthCallbackStatus, OAuthProvider, type AuthJsonAddRequest } from "@/api/endpoints/oauthProvider"
import { useTranslations } from "next-intl"
import { RefreshCw, X, Plus, ExternalLink, Loader2 } from "lucide-react"
import { cn } from "@/lib/utils"
import { toast } from "@/components/common/Toast"

interface CreateEditOAuthProviderModalProps {
    open: boolean
    onOpenChange: (open: boolean) => void
    provider: OAuthProvider | null
}

interface AuthJsonFormItem {
    id?: number
    enabled: boolean
    content: string
    remark: string
    isNew?: boolean
}

interface FormData {
    name: string
    provider_type: string
    status: string
    model: string
    custom_model: string
    match_regex: string
    auth_jsons: AuthJsonFormItem[]
}

// Auth JSON placeholders for different provider types
const authJsonPlaceholders: Record<string, string> = {
    iflow: '{"BXAuth": "your_bxauth_cookie_value"}',
    kiro: '{"refreshToken": "your_refresh_token", "region": "us-east-1"}',
}

// Provider types that use OAuth flow (not AuthJson)
const oauthProviderTypes = ["codex"] as const
type OAuthProviderType = typeof oauthProviderTypes[number]

// Check if provider type uses OAuth flow
function isOAuthProvider(type: string): type is OAuthProviderType {
    return oauthProviderTypes.includes(type as OAuthProviderType)
}

export function CreateEditOAuthProviderModal({
    open,
    onOpenChange,
    provider,
}: CreateEditOAuthProviderModalProps) {
    const createMutation = useCreateOAuthProvider()
    const updateMutation = useUpdateOAuthProvider()
    const fetchModel = useFetchOAuthModel()
    const getAuthURL = useGetOAuthAuthURL()
    const handleCallback = useHandleOAuthCallback()
    const t = useTranslations("oauthProvider")
    const tChannel = useTranslations("channel.form")

    // OAuth flow state
    const [oauthStep, setOauthStep] = useState<"init" | "authorizing" | "callback">("init")
    const [oauthState, setOauthState] = useState<string | null>(null)
    const [oauthCallbackMode, setOauthCallbackMode] = useState<"auto" | "manual">("auto")
    const [oauthCallbackUrl, setOauthCallbackUrl] = useState("")
    const [oauthProviderName, setOauthProviderName] = useState("")

    // Poll for OAuth callback status in auto mode
    const callbackStatus = useOAuthCallbackStatus(oauthCallbackMode === "auto" ? oauthState : null)

    // Handle auto mode completion
    useEffect(() => {
        if (
            callbackStatus.data &&
            "status" in callbackStatus.data &&
            callbackStatus.data.status === "completed" &&
            "provider" in callbackStatus.data
        ) {
            onOpenChange(false)
            reset()
        }
    }, [callbackStatus.data, onOpenChange])

    const { register, handleSubmit, reset, setValue, watch } = useForm<FormData>({
        defaultValues: {
            name: "",
            provider_type: "iflow",
            status: "1",
            model: "",
            custom_model: "",
            match_regex: "",
            auth_jsons: [{ enabled: true, content: "", remark: "", isNew: true }],
        },
    })

    const autoModels = watch("model")
        ? watch("model").split(',').map((m) => m.trim()).filter(Boolean)
        : [];
    const customModels = watch("custom_model")
        ? watch("custom_model").split(',').map((m) => m.trim()).filter(Boolean)
        : [];
    const matchRegex = watch("match_regex") || "";
    const providerType = watch("provider_type") || "iflow";
    const authJsons = watch("auth_jsons") || [];
    const [inputValue, setInputValue] = useState('')
    const inputRef = useRef<HTMLInputElement>(null)

    const updateModels = (nextAuto: string[], nextCustom: string[]) => {
        const model = nextAuto.join(',')
        const custom_model = nextCustom.join(',')
        setValue("model", model)
        setValue("custom_model", custom_model)
    }

    const filterModelsByRegex = (models: string[]): string[] => {
        if (!matchRegex.trim()) return models
        try {
            const regex = new RegExp(matchRegex, 'i')
            return models.filter(m => regex.test(m))
        } catch {
            return models
        }
    }

    const handleRefreshModels = async () => {
        if (!provider?.id) {
            toast.warning(t("modelRefreshNoProvider"))
            return
        }
        fetchModel.mutate(provider.id, {
            onSuccess: (data) => {
                if (data && data.length > 0) {
                    const filtered = filterModelsByRegex(data)
                    if (filtered.length > 0) {
                        const nextAuto = Array.from(new Set([...autoModels, ...filtered].map((m) => m.trim()).filter(Boolean)))
                        updateModels(nextAuto, customModels)
                        toast.success(t("modelRefreshSuccess"))
                    } else {
                        toast.warning(t("modelRefreshEmpty"))
                    }
                } else {
                    toast.warning(t("modelRefreshEmpty"))
                }
            },
            onError: (error) => {
                const errorMessage = error instanceof Error ? error.message : String(error)
                toast.error(t("modelRefreshFailed"), { description: errorMessage })
            },
        })
    }

    const handleAddModel = (model: string) => {
        const trimmedModel = model.trim()
        if (trimmedModel && !customModels.includes(trimmedModel) && !autoModels.includes(trimmedModel)) {
            updateModels(autoModels, [...customModels, trimmedModel])
        }
        setInputValue('')
    }

    const handleRemoveAutoModel = (model: string) => {
        updateModels(autoModels.filter(m => m !== model), customModels)
    }

    const handleRemoveCustomModel = (model: string) => {
        updateModels(autoModels, customModels.filter(m => m !== model))
    }

    const handleInputKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
        if (e.key === "Enter") {
            e.preventDefault()
            if (inputValue.trim()) handleAddModel(inputValue)
        }
    }

    // AuthJson management functions (similar to ChannelKey pattern)
    const handleAddAuthJson = () => {
        const current = watch("auth_jsons") || []
        setValue("auth_jsons", [...current, { enabled: true, content: "", remark: "", isNew: true }])
    }

    const handleUpdateAuthJson = (idx: number, patch: Partial<AuthJsonFormItem>) => {
        const current = watch("auth_jsons") || []
        const next = current.map((aj, i) => (i === idx ? { ...aj, ...patch } : aj))
        setValue("auth_jsons", next)
    }

    const handleRemoveAuthJson = (idx: number) => {
        const current = watch("auth_jsons") || []
        if (current.length <= 1) return
        setValue("auth_jsons", current.filter((_, i) => i !== idx))
    }

    // OAuth flow handlers
    const handleStartOAuth = async () => {
        try {
            const result = await getAuthURL.mutateAsync({
                type: providerType,
            })
            setOauthState(result.state)
            setOauthCallbackMode(result.callback_mode)
            setOauthStep("authorizing")
            window.open(result.auth_url, "_blank")
            toast.info(result.instructions)
        } catch (error) {
            toast.error("Failed to start OAuth flow")
        }
    }

    const handleManualOAuthCallback = async () => {
        if (!oauthCallbackUrl.trim()) {
            toast.error("Please paste the callback URL")
            return
        }
        try {
            const result = await handleCallback.mutateAsync({
                callback_url: oauthCallbackUrl,
                name: oauthProviderName || undefined,
            })
            onOpenChange(false)
            reset()
        } catch (error) {
            // Error handled by mutation
        }
    }

    const resetOAuth = () => {
        setOauthStep("init")
        setOauthState(null)
        setOauthCallbackUrl("")
        setOauthProviderName("")
    }

    useEffect(() => {
        if (open) {
            if (provider) {
                setValue("name", provider.name)
                setValue("provider_type", provider.provider_type)
                setValue("status", String(provider.status))
                setValue("model", provider.channel?.model || "")
                setValue("custom_model", provider.channel?.custom_model || "")
                setValue("match_regex", provider.channel?.match_regex || "")
                // Load existing auth_jsons or empty one
                if (provider.auth_jsons && provider.auth_jsons.length > 0) {
                    setValue("auth_jsons", provider.auth_jsons.map(aj => ({
                        id: aj.id,
                        enabled: aj.enabled,
                        content: aj.content,
                        remark: aj.remark || "",
                        isNew: false,
                    })))
                } else {
                    setValue("auth_jsons", [{ enabled: true, content: "", remark: "", isNew: true }])
                }
            } else {
                reset({
                    name: "",
                    provider_type: "iflow",
                    status: "1",
                    model: "",
                    custom_model: "",
                    match_regex: "",
                    auth_jsons: [{ enabled: true, content: "", remark: "", isNew: true }],
                })
            }
        }
    }, [open, provider, setValue, reset])

    const onSubmit = (data: FormData) => {
        const status = parseInt(data.status)

        // Filter out empty auth_jsons for create
        const validAuthJsons = data.auth_jsons.filter(aj => aj.content.trim() !== "")

        if (provider) {
            // Build update request
            const authJsonsToAdd: AuthJsonAddRequest[] = []
            const authJsonsToUpdate: { id: number; enabled?: boolean; content?: string; remark?: string }[] = []
            const authJsonsToDelete: number[] = []

            // Track which existing auth_jsons should be updated
            const existingIds = new Set(provider.auth_jsons?.map(aj => aj.id) || [])
            const formIds = new Set(data.auth_jsons.filter(aj => aj.id).map(aj => aj.id!))

            // Find deleted auth_jsons
            for (const existingId of existingIds) {
                if (!formIds.has(existingId)) {
                    authJsonsToDelete.push(existingId)
                }
            }

            // Process form auth_jsons
            for (const aj of validAuthJsons) {
                if (aj.isNew || !aj.id) {
                    authJsonsToAdd.push({
                        enabled: aj.enabled,
                        content: aj.content,
                        remark: aj.remark || undefined,
                    })
                } else {
                    authJsonsToUpdate.push({
                        id: aj.id,
                        enabled: aj.enabled,
                        content: aj.content,
                        remark: aj.remark || undefined,
                    })
                }
            }

            updateMutation.mutate(
                {
                    id: provider.id,
                    name: data.name,
                    status: status,
                    model: data.model,                                                                       
                    custom_model: data.custom_model,
                    match_regex: data.match_regex || undefined,
                    auth_jsons_to_add: authJsonsToAdd.length > 0 ? authJsonsToAdd : undefined,
                    auth_jsons_to_update: authJsonsToUpdate.length > 0 ? authJsonsToUpdate : undefined,
                    auth_jsons_to_delete: authJsonsToDelete.length > 0 ? authJsonsToDelete : undefined,
                },
                {
                    onSuccess: () => {
                        onOpenChange(false)
                        reset()
                    },
                }
            )
        } else {
            // Create new provider
            createMutation.mutate(
                {
                    name: data.name,
                    provider_type: data.provider_type,
                    status: status,
                    model: data.model,
                    custom_model: data.custom_model,
                    match_regex: data.match_regex || undefined,
                    auth_jsons: validAuthJsons.map(aj => ({
                        enabled: aj.enabled,
                        content: aj.content,
                        remark: aj.remark || undefined,
                    })),
                },
                {
                    onSuccess: () => {
                        onOpenChange(false)
                        reset()
                    },
                }
            )
        }
    }

    const isLoading = createMutation.isPending || updateMutation.isPending

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="sm:max-w-[800px]">
                <DialogHeader>
                    <DialogTitle>{provider ? t("editTitle") : t("createTitle")}</DialogTitle>
                </DialogHeader>
                <form onSubmit={handleSubmit(onSubmit)} className="space-y-4 py-4">
                    <div className="space-y-2">
                        <Label htmlFor="name">{t("name")}</Label>
                        <Input id="name" {...register("name", { required: true })} placeholder={t("namePlaceholder")} />
                    </div>
                    <div className="space-y-2">
                        <Label>{t("type")}</Label>
                        <div className="flex gap-1">
                            {(["iflow", "kiro", "codex"] as const).map((type) => (
                                <button
                                    key={type}
                                    type="button"
                                    onClick={() => {
                                        setValue("provider_type", type)
                                        if (isOAuthProvider(type)) {
                                            resetOAuth()
                                        }
                                    }}
                                    className={cn(
                                        "flex-1 py-1.5 text-sm rounded-lg transition-colors",
                                        providerType === type
                                            ? "bg-primary text-primary-foreground"
                                            : "bg-muted hover:bg-muted/80"
                                    )}
                                >
                                    {type === "iflow" ? "iFlow" : type === "kiro" ? "Kiro" : "Codex"}
                                </button>
                            ))}
                        </div>
                    </div>

                    {/* Auth JSON management section or OAuth flow */}
                    {isOAuthProvider(providerType) ? (
                        // OAuth flow for Codex
                        <div className="space-y-4">
                            <Label>Codex OAuth</Label>
                            {oauthStep === "init" && (
                                <div className="space-y-3">
                                    <p className="text-sm text-muted-foreground">
                                        Click the button below to start OAuth authorization. You will be redirected to OpenAI to log in.
                                    </p>
                                    <div className="space-y-2">
                                        <Label>Provider Name (optional)</Label>
                                        <Input
                                            value={oauthProviderName}
                                            onChange={(e) => setOauthProviderName(e.target.value)}
                                            placeholder="My Codex Provider"
                                        />
                                    </div>
                                    <Button
                                        type="button"
                                        onClick={handleStartOAuth}
                                        disabled={getAuthURL.isPending}
                                        className="w-full"
                                    >
                                        {getAuthURL.isPending ? (
                                            <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                                        ) : (
                                            <ExternalLink className="h-4 w-4 mr-2" />
                                        )}
                                        Start OAuth Authorization
                                    </Button>
                                </div>
                            )}
                            {oauthStep === "authorizing" && (
                                <div className="space-y-3">
                                    <div className="rounded-md bg-blue-50 p-3 dark:bg-blue-950">
                                        <p className="text-sm text-blue-700 dark:text-blue-300">
                                            {oauthCallbackMode === "auto" ? (
                                                <>
                                                    <strong>Automatic mode:</strong> Waiting for authorization...
                                                    <br />
                                                    <span className="text-xs">The page will update automatically when complete.</span>
                                                </>
                                            ) : (
                                                <>
                                                    <strong>Manual mode:</strong> After authorizing, copy the URL from your browser and paste it below.
                                                </>
                                            )}
                                        </p>
                                    </div>
                                    {oauthCallbackMode === "auto" && (
                                        <div className="flex items-center gap-2 text-sm text-muted-foreground">
                                            <Loader2 className="h-4 w-4 animate-spin" />
                                            Polling for authorization...
                                        </div>
                                    )}
                                    {oauthCallbackMode === "manual" && (
                                        <>
                                            <Input
                                                value={oauthCallbackUrl}
                                                onChange={(e) => setOauthCallbackUrl(e.target.value)}
                                                placeholder="http://localhost:xxxx/callback?code=xxx&state=xxx"
                                            />
                                            <Button
                                                type="button"
                                                onClick={handleManualOAuthCallback}
                                                disabled={handleCallback.isPending || !oauthCallbackUrl.trim()}
                                                className="w-full"
                                            >
                                                {handleCallback.isPending && (
                                                    <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                                                )}
                                                Complete Login
                                            </Button>
                                        </>
                                    )}
                                    <Button
                                        type="button"
                                        variant="outline"
                                        onClick={resetOAuth}
                                        className="w-full"
                                    >
                                        Cancel
                                    </Button>
                                </div>
                            )}
                        </div>
                    ) : (
                        // AuthJson for iFlow/Kiro
                        <div className="space-y-2">
                            <div className="flex items-center justify-between">
                                <Label>{t("authJson")} {authJsons.length > 0 ? `(${authJsons.length})` : ''}</Label>
                                <Button
                                    type="button"
                                    variant="ghost"
                                    size="sm"
                                    onClick={handleAddAuthJson}
                                    className="h-6 px-2 text-xs text-muted-foreground/70 hover:text-muted-foreground hover:bg-transparent"
                                >
                                    <Plus className="h-3 w-3 mr-1" />
                                    {tChannel("add")}
                                </Button>
                            </div>
                            <div className="space-y-2">
                                {authJsons.map((aj, idx) => (
                                    <div key={aj.id ?? `new-${idx}`} className="flex flex-col gap-2 p-3 border rounded-xl bg-muted/20">
                                        <div className="flex items-center gap-2">
                                            <Textarea
                                                value={aj.content}
                                                onChange={(e) => handleUpdateAuthJson(idx, { content: e.target.value })}
                                                placeholder={authJsonPlaceholders[providerType] || t("authJsonPlaceholder")}
                                                className="min-h-[60px] font-mono text-sm flex-1"
                                            />
                                        </div>
                                        <div className="flex items-center gap-2">
                                            <Input
                                                type="text"
                                                value={aj.remark}
                                                onChange={(e) => handleUpdateAuthJson(idx, { remark: e.target.value })}
                                                placeholder={tChannel("remark")}
                                                className="rounded-xl flex-1"
                                            />
                                            <div className="flex items-center gap-2">
                                                <Switch
                                                    checked={aj.enabled}
                                                    onCheckedChange={(checked) => handleUpdateAuthJson(idx, { enabled: checked })}
                                                />
                                            </div>
                                            <Button
                                                type="button"
                                                variant="ghost"
                                                size="sm"
                                                onClick={() => handleRemoveAuthJson(idx)}
                                                disabled={authJsons.length <= 1}
                                                className="h-8 w-8 p-0 rounded-xl text-muted-foreground hover:text-destructive hover:bg-transparent disabled:opacity-40"
                                                title="Remove"
                                            >
                                                <X className="h-4 w-4" />
                                            </Button>
                                        </div>
                                    </div>
                                ))}
                            </div>
                        </div>
                    )}

                    {/* Model management section */}
                    <div className="space-y-2">
                        <div className="flex items-center justify-between">
                            <Label>{tChannel("model")}</Label>
                            <Button
                                type="button"
                                variant="ghost"
                                size="sm"
                                onClick={handleRefreshModels}
                                disabled={!provider?.id || fetchModel.isPending}
                                className="h-6 px-2 text-xs text-muted-foreground/50 hover:text-muted-foreground hover:bg-transparent"
                            >
                                <RefreshCw className={`h-3 w-3 mr-1 ${fetchModel.isPending ? 'animate-spin' : ''}`} />
                                {tChannel("modelRefresh")}
                            </Button>
                        </div>
                        <input type="hidden" {...register("model")} />
                        <input type="hidden" {...register("custom_model")} />

                        <div className="relative">
                            <Input
                                ref={inputRef}
                                type="text"
                                value={inputValue}
                                onChange={(e) => setInputValue(e.target.value)}
                                onKeyDown={handleInputKeyDown}
                                placeholder={tChannel("modelCustomPlaceholder")}
                                className="pr-10 rounded-xl"
                            />
                            {inputValue.trim() && !customModels.includes(inputValue.trim()) && !autoModels.includes(inputValue.trim()) && (
                                <Button
                                    type="button"
                                    variant="ghost"
                                    size="sm"
                                    onClick={() => handleAddModel(inputValue)}
                                    className="absolute rounded-lg right-1 top-1/2 -translate-y-1/2 h-7 w-7 p-0 text-muted-foreground hover:bg-accent hover:text-accent-foreground transition-colors"
                                    title={tChannel("modelAdd")}
                                >
                                    <Plus className="size-4" />
                                </Button>
                            )}
                        </div>

                        <div className="space-y-2">
                            <div className="flex items-center justify-between">
                                <label className="text-xs font-medium text-card-foreground">
                                    {tChannel("modelSelected")} {(autoModels.length + customModels.length) > 0 && `(${autoModels.length + customModels.length})`}
                                </label>
                                {(autoModels.length + customModels.length) > 0 && (
                                    <Button
                                        type="button"
                                        variant="ghost"
                                        size="sm"
                                        onClick={() => {
                                            updateModels([], [])
                                        }}
                                        className="h-6 px-2 text-xs text-muted-foreground/50 hover:text-muted-foreground hover:bg-transparent"
                                    >
                                        {tChannel("modelClearAll")}
                                    </Button>
                                )}
                            </div>
                            <div className="rounded-xl border border-border bg-muted/30 p-2.5 max-h-40 min-h-12 overflow-y-auto">
                                {(autoModels.length + customModels.length) > 0 ? (
                                    <div className="flex flex-wrap gap-1.5">
                                        {autoModels.map((model) => (
                                            <Badge key={model} variant="secondary" className="bg-muted hover:bg-muted/80">
                                                {model}
                                                <button
                                                    type="button"
                                                    onClick={() => handleRemoveAutoModel(model)}
                                                    className="ml-1 rounded-sm opacity-70 hover:opacity-100 focus:outline-none focus:ring-1 focus:ring-ring"
                                                >
                                                    <X className="h-3 w-3" />
                                                </button>
                                            </Badge>
                                        ))}
                                        {customModels.map((model) => (
                                            <Badge key={model} className="bg-primary hover:bg-primary/90">
                                                {model}
                                                <button
                                                    type="button"
                                                    onClick={() => handleRemoveCustomModel(model)}
                                                    className="ml-1 rounded-sm opacity-70 hover:opacity-100 focus:outline-none focus:ring-1 focus:ring-ring"
                                                >
                                                    <X className="h-3 w-3" />
                                                </button>
                                            </Badge>
                                        ))}
                                    </div>
                                ) : (
                                    <div className="flex items-center justify-center h-8 text-xs text-muted-foreground">
                                        {tChannel("modelNoSelected")}
                                    </div>
                                )}
                            </div>
                        </div>
                    </div>

                    {/* Match Regex - directly visible */}
                    <div className="space-y-2">
                        <Label htmlFor="match_regex">{tChannel("matchRegex")}</Label>
                        <Input
                            id="match_regex"
                            type="text"
                            {...register("match_regex")}
                            placeholder={tChannel("matchRegexPlaceholder")}
                            className="rounded-xl"
                        />
                    </div>

                    {/* Enabled toggle */}
                    <div className="flex items-center justify-between">
                        <Label htmlFor="enabled">{t("enabled")}</Label>
                        <Switch
                            id="enabled"
                            checked={watch("status") === "1"}
                            onCheckedChange={(checked) => setValue("status", checked ? "1" : "0")}
                        />
                    </div>
                    <DialogFooter>
                        <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                            {t("cancel")}
                        </Button>
                        <Button type="submit" disabled={isLoading}>
                            {isLoading ? t("saving") : (provider ? t("update") : t("create"))}
                        </Button>
                    </DialogFooter>
                </form>
            </DialogContent>
        </Dialog>
    )
}
