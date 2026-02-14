
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
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "@/components/ui/select"
import { useForm } from "react-hook-form"
import { useEffect } from "react"
import { useCreateOAuthProvider, useUpdateOAuthProvider, OAuthProvider } from "@/api/endpoints/oauthProvider"
import { useTranslations } from "next-intl"

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
}

export function CreateEditOAuthProviderModal({
    open,
    onOpenChange,
    provider,
}: CreateEditOAuthProviderModalProps) {
    const createMutation = useCreateOAuthProvider()
    const updateMutation = useUpdateOAuthProvider()
    const t = useTranslations("oauthProvider")

    const { register, handleSubmit, reset, setValue, watch } = useForm<FormData>({
        defaultValues: {
            name: "",
            provider_type: "iflow",
            cookie: "",
            status: "1",
        },
    })

    useEffect(() => {
        if (open) {
            if (provider) {
                setValue("name", provider.name)
                setValue("provider_type", provider.provider_type)
                setValue("cookie", "") // Cookie is not returned by API for security
                setValue("status", String(provider.status))
            } else {
                reset({
                    name: "",
                    provider_type: "iflow",
                    cookie: "",
                    status: "1",
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
