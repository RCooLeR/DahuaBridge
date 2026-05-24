from __future__ import annotations

LANGUAGE_AUTO = "auto"
LANGUAGE_EN = "en"
LANGUAGE_UK = "uk"

LANGUAGE_OPTIONS = {
    LANGUAGE_AUTO: "Auto (Home Assistant)",
    LANGUAGE_EN: "English",
    LANGUAGE_UK: "Українська",
}

LABELS = {
    LANGUAGE_EN: {
        "camera": "Camera",
        "online": "Online",
        "probe_now": "Probe Now",
        "refresh_inventory": "Refresh Inventory",
        "answer_call": "Answer Call",
        "hangup_call": "Hang Up Call",
        "reset_bridge_session": "Reset Bridge Session",
        "enable_rtp_export": "Enable RTP Export",
        "disable_rtp_export": "Disable RTP Export",
        "unlock": "Unlock {index}",
        "auto_record_enabled": "Auto Record",
    },
    LANGUAGE_UK: {
        "camera": "Камера",
        "online": "Онлайн",
        "probe_now": "Опитати зараз",
        "refresh_inventory": "Оновити інвентар",
        "answer_call": "Відповісти на виклик",
        "hangup_call": "Завершити виклик",
        "reset_bridge_session": "Скинути сесію Bridge",
        "enable_rtp_export": "Увімкнути RTP експорт",
        "disable_rtp_export": "Вимкнути RTP експорт",
        "unlock": "Відкрити {index}",
        "auto_record_enabled": "Автозапис",
    },
}

FIELD_NAMES = {
    LANGUAGE_EN: {
        "bridge_forward_errors": "Bridge Forward Errors",
        "bridge_forwarded_packets": "Bridge Forwarded Packets",
        "bridge_session_active": "Bridge Session Active",
        "bridge_session_count": "Bridge Session Count",
        "bridge_uplink_active": "Bridge Uplink Active",
        "bridge_uplink_codec": "Bridge Uplink Codec",
        "bridge_uplink_packets": "Bridge Uplink Packets",
        "call_state": "Call State",
        "channel_index": "Channel Index",
        "configured_external_uplink_target_count": "Configured RTP Export Targets",
        "disk_fault": "Disk Fault",
        "direct_ipc_configured": "Direct IPC Configured",
        "direct_ipc_configured_ip": "Direct IPC Configured IP",
        "direct_ipc_http_port": "Direct IPC HTTP Port",
        "direct_ipc_https_port": "Direct IPC HTTPS Port",
        "direct_ipc_inventory_username": "Direct IPC Inventory Username",
        "direct_ipc_ip": "Direct IPC Address",
        "direct_ipc_model": "Direct IPC Model",
        "direct_ipc_rtsp_port": "Direct IPC RTSP Port",
        "external_uplink_enabled": "RTP Export Enabled",
        "last_call_duration_seconds": "Last Call Duration",
        "last_call_ended_at": "Last Call Ended At",
        "last_call_source": "Last Call Source",
        "last_call_started_at": "Last Call Started At",
        "last_ring_at": "Last Ring At",
        "main_codec": "Main Codec",
        "main_resolution": "Main Resolution",
        "nvr_config_reason": "NVR Config Write Reason",
        "nvr_config_writable": "NVR Config Writable",
        "onvif_h264_available": "ONVIF H264 Available",
        "onvif_profile_name": "ONVIF Profile Name",
        "onvif_profile_token": "ONVIF Profile Token",
        "recommended_ha_integration": "Recommended HA Integration",
        "recommended_ha_reason": "Recommended HA Reason",
        "recommended_profile": "Recommended Profile",
        "stream_available": "Stream Available",
        "sub_codec": "Sub Codec",
        "sub_resolution": "Sub Resolution",
        "used_percent": "Storage Used Percent",
    },
    LANGUAGE_UK: {
        "bridge_forward_errors": "Помилки пересилання Bridge",
        "bridge_forwarded_packets": "Переслані пакети Bridge",
        "bridge_session_active": "Активна сесія Bridge",
        "bridge_session_count": "Кількість сесій Bridge",
        "bridge_uplink_active": "Активний висхідний потік Bridge",
        "bridge_uplink_codec": "Кодек висхідного потоку Bridge",
        "bridge_uplink_packets": "Пакети висхідного потоку Bridge",
        "call_state": "Стан виклику",
        "channel_index": "Індекс каналу",
        "configured_external_uplink_target_count": "Налаштовані цілі RTP експорту",
        "disk_fault": "Помилка диска",
        "direct_ipc_configured": "Прямий IPC налаштовано",
        "direct_ipc_configured_ip": "Налаштована IP адреса прямого IPC",
        "direct_ipc_http_port": "HTTP порт прямого IPC",
        "direct_ipc_https_port": "HTTPS порт прямого IPC",
        "direct_ipc_inventory_username": "Користувач інвентаря прямого IPC",
        "direct_ipc_ip": "Адреса прямого IPC",
        "direct_ipc_model": "Модель прямого IPC",
        "direct_ipc_rtsp_port": "RTSP порт прямого IPC",
        "external_uplink_enabled": "RTP експорт увімкнено",
        "last_call_duration_seconds": "Тривалість останнього виклику",
        "last_call_ended_at": "Останній виклик завершено",
        "last_call_source": "Джерело останнього виклику",
        "last_call_started_at": "Останній виклик розпочато",
        "last_ring_at": "Останній дзвінок",
        "main_codec": "Основний кодек",
        "main_resolution": "Основна роздільна здатність",
        "nvr_config_reason": "Причина запису конфігурації NVR",
        "nvr_config_writable": "Конфігурація NVR доступна для запису",
        "onvif_h264_available": "ONVIF H264 доступний",
        "onvif_profile_name": "Назва профілю ONVIF",
        "onvif_profile_token": "Токен профілю ONVIF",
        "recommended_ha_integration": "Рекомендована HA інтеграція",
        "recommended_ha_reason": "Причина рекомендації HA",
        "recommended_profile": "Рекомендований профіль",
        "stream_available": "Потік доступний",
        "sub_codec": "Додатковий кодек",
        "sub_resolution": "Додаткова роздільна здатність",
        "used_percent": "Використано сховища",
    },
}


def normalize_language_choice(raw: object, default: str = LANGUAGE_AUTO) -> str:
    value = str(raw or "").strip().lower()
    if value in LANGUAGE_OPTIONS:
        return value
    prefix = value.replace("_", "-").split("-", 1)[0]
    if prefix in {LANGUAGE_EN, LANGUAGE_UK}:
        return prefix
    if value in {"ua", "ukr", "ukrainian", "українська", "украинский"}:
        return LANGUAGE_UK
    if value in {"eng", "english"}:
        return LANGUAGE_EN
    return default


def resolve_language(raw: object, home_assistant_language: object = None) -> str:
    selected = normalize_language_choice(raw)
    if selected != LANGUAGE_AUTO:
        return selected

    detected = normalize_language_choice(home_assistant_language, LANGUAGE_EN)
    return detected if detected != LANGUAGE_AUTO else LANGUAGE_EN


def localized_label(key: str, language: str, **kwargs: object) -> str:
    labels = LABELS.get(language, LABELS[LANGUAGE_EN])
    template = labels.get(key, LABELS[LANGUAGE_EN].get(key, key))
    return template.format(**kwargs)


def localized_field_name(field: str, language: str) -> str:
    names = FIELD_NAMES.get(language, FIELD_NAMES[LANGUAGE_EN])
    return names.get(field, FIELD_NAMES[LANGUAGE_EN].get(field, _fallback_field_name(field)))


def _fallback_field_name(field: str) -> str:
    return field.replace("_", " ").title()
