import { Channel, ChannelType } from '@/api/endpoints/channel';
import { useTranslations } from 'next-intl';

export function useChannelName() {
    const t = useTranslations('channel.form');

    const channelTypeMap: Record<number, string> = {
        [ChannelType.OpenAIChat]: 'typeOpenAIChat',
        [ChannelType.OpenAIResponse]: 'typeOpenAIResponse',
        [ChannelType.Anthropic]: 'typeAnthropic',
        [ChannelType.Gemini]: 'typeGemini',
        [ChannelType.Volcengine]: 'typeVolcengine',
        [ChannelType.OpenAIEmbedding]: 'typeOpenAIEmbedding',
    };

    const getChannelNameByType = (name: string | undefined, type: number) => {
        if (name && name.trim()) {
            return name;
        }

        const key = channelTypeMap[type];
        return key ? t(key) : 'Unknown';
    };

    const getChannelName = (channel: Channel) => {
        return getChannelNameByType(channel.name, channel.type);
    };

    return { getChannelName, getChannelNameByType };
}
