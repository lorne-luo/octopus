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
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "@/components/ui/select"
import { useForm } from "react-hook-form"
import { useEffect, useState, useRef } from "react"
import { useCreateOAuthProvider, useUpdateOAuthProvider, useFetchOAuthModel, OAuthProvider } from "@/api/endpoints/oauthProvider"
import { useTranslations } from "next-intl"
import { RefreshCw, X, Plus } from "lucide-react"
import { toast } from "@/components/common/Toast"

interface CreateEditOAuthProviderModalProps {
    open: boolean
    onOpenChange: (open: boolean) => void
    provider: OAuthProvider | null
}

interface FormData {
    name: string
    provider_type: string
    cookie: string
    status: string
    model: string
    custom_model: string
}

export function CreateEditOAuthProviderModal({
    open,
    onOpenChange,
    provider,
}: CreateEditOAuthProviderModalProps) {
    const createMutation = useCreateOAuthProvider()
    const updateMutation = useUpdateOAuthProvider()
    const fetchModel = useFetchOAuthModel()
    const t = useTranslations("oauthProvider")

    const { register, handleSubmit, reset, setValue, watch } = useForm<FormData>({
        defaultValues: {
            name: "",
            provider_type: "iflow",
            cookie: "",
            status: "1",
            model: "",
            custom_model: "",
        },
    })

    const autoModels = watch("model")
        ? watch("model").split(',').map((m) => m.trim()).filter(Boolean)
        : [];
    const customModels = watch("custom_model")
        ? watch("custom_model").split(',').map((m) => m.trim()).filter(Boolean)
        : [];
    const [inputValue, setInputValue] = useState('')
    const inputRef = useRef<HTMLInputElement>(null)

    const updateModels = (nextAuto: string[], nextCustom: string[]) => {
        const model = nextAuto.join(',')
        const custom_model = nextCustom.join(',')
        setValue("model", model)
        setValue("custom_model", custom_model)
    }

    const handleRefreshModels = async () => {
        if (!provider?.id) {
            toast.warning(t("modelRefreshNoProvider"))
            return
        }
        fetchModel.mutate(provider.id, {
            onSuccess: (data) => {
                if (data && data.length > 0) {
                    const nextAuto = Array.from(new Set([...autoModels, ...data].map((m) => m.trim()).filter(Boolean)))
                    updateModels(nextAuto, customModels)
                    toast.success(t("modelRefreshSuccess"))
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

    useEffect(() => {
        if (open) {
            if (provider) {
                setValue("name", provider.name)
                setValue("provider_type", provider.provider_type)
                setValue("cookie", "")
                setValue("status", String(provider.status))
                setValue("model", provider.model || "")
                setValue("custom_model", provider.custom_model || "")
            } else {
                reset({
                    name: "",
                    provider_type: "iflow",
                    cookie: "",
                    status: "1",
                    model: "",
                    custom_model: "",
                })
            }
        }
    }, [open, provider, setValue, reset])

    const onSubmit = (data: FormData) => {
        const status = parseInt(data.status)
        if (provider) {
            updateMutation.mutate(
                {
                    id: provider.id,
                    name: data.name,
                    provider_type: data.provider_type,
                    cookie: data.cookie || undefined,
                    status: status,
                    model: data.model || undefined,
                    custom_model: data.custom_model || undefined,
                },
                {
                    onSuccess: () => {
                        onOpenChange(false)
                        reset()
                    },
                }
            )
        } else {
            createMutation.mutate(
                {
                    name: data.name,
                    provider_type: data.provider_type,
                    cookie: data.cookie,
                    status: status,
                    model: data.model,
                    custom_model: data.custom_model,
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
            <DialogContent className="sm:max-w-[500px]">
                <DialogHeader>
                    <DialogTitle>{provider ? t("editTitle") : t("createTitle")}</DialogTitle>
                </DialogHeader>
                <form onSubmit={handleSubmit(onSubmit)} className="space-y-4 py-4">
                    <div className="space-y-2">
                        <Label htmlFor="name">{t("name")}</Label>
                        <Input id="name" {...register("name", { required: true })} placeholder={t("namePlaceholder")} />
                    </div>
                    <div className="space-y-2">
                        <Label htmlFor="provider_type">{t("type")}</Label>
                        <Select
                            onValueChange={(value) => setValue("provider_type", value)}
                            defaultValue={watch("provider_type")}
                        >
                            <SelectTrigger>
                                <SelectValue placeholder="Select type" />
                            </SelectTrigger>
                            <SelectContent>
                                <SelectItem value="iflow">IFlow</SelectItem>
                                {/* Add more types here if needed */}
                            </SelectContent>
                        </Select>
                    </div>
                    <div className="space-y-2">
                        <Label htmlFor="cookie">{t("cookie")}</Label>
                        <Textarea
                            id="cookie"
                            {...register("cookie", { required: !provider })}
                            placeholder={provider ? t("cookiePlaceholderEdit") : t("cookiePlaceholder")}
                            className="min-h-[100px]"
                        />
                    </div>
                    <div className="space-y-2">
                        <div className="flex items-center justify-between">
                            <Label>{t("model")}</Label>
                            <Button
                                type="button"
                                variant="ghost"
                                size="sm"
                                onClick={handleRefreshModels}
                                disabled={!provider?.id || fetchModel.isPending}
                                className="h-6 px-2 text-xs text-muted-foreground/50 hover:text-muted-foreground hover:bg-transparent"
                            >
                                <RefreshCw className={`h-3 w-3 mr-1 ${fetchModel.isPending ? 'animate-spin' : ''}`} />
                                {t("modelRefresh")}
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
                                placeholder={t("modelCustomPlaceholder")}
                                className="pr-10 rounded-xl"
                            />
                            {inputValue.trim() && !customModels.includes(inputValue.trim()) && !autoModels.includes(inputValue.trim()) && (
                                <Button
                                    type="button"
                                    variant="ghost"
                                    size="sm"
                                    onClick={() => handleAddModel(inputValue)}
                                    className="absolute rounded-lg right-1 top-1/2 -translate-y-1/2 h-7 w-7 p-0 text-muted-foreground hover:bg-accent hover:text-accent-foreground transition-colors"
                                    title={t("modelAdd")}
                                >
                                    <Plus className="size-4" />
                                </Button>
                            )}
                        </div>

                        <div className="space-y-2">
                            <div className="flex items-center justify-between">
                                <label className="text-xs font-medium text-card-foreground">
                                    {t("modelSelected")} {(autoModels.length + customModels.length) > 0 && `(${autoModels.length + customModels.length})`}
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
                                        {t("modelClearAll")}
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
                                        {t("modelNoSelected")}
                                    </div>
                                )}
                            </div>
                        </div>
                    </div>
                    <div className="space-y-2">
                         <Label htmlFor="status">{t("status")}</Label>
                        <Select
                            onValueChange={(value) => setValue("status", value)}
                            defaultValue={watch("status")}
                        >
                            <SelectTrigger>
                                <SelectValue placeholder="Select status" />
                            </SelectTrigger>
                            <SelectContent>
                                <SelectItem value="1">{t("active")}</SelectItem>
                                <SelectItem value="0">{t("disabled")}</SelectItem>
                            </SelectContent>
                        </Select>
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