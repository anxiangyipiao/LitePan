<script setup lang="ts">
import { ref } from "vue";
import { getApiErrorMessage } from "@/api/client";
import { testQB } from "@/api/qb";
import { toast } from "@/composables/useToast";
import SettingsCard from "@/components/admin/SettingsCard.vue";
import SettingsRow from "@/components/admin/SettingsRow.vue";
import SettingsHelpTooltip from "@/components/admin/SettingsHelpTooltip.vue";
import AppInput from "@/components/base/AppInput.vue";
import AppButton from "@/components/base/AppButton.vue";

const QB_KEYS = ["qb_url", "qb_username", "qb_password", "qb_save_path", "qb_category"] as const;

const props = withDefaults(
  defineProps<{
    form: Record<string, string>;
    isChanged: (key: string | { key: string } | any) => boolean;
    accent?: string;
  }>(),
  { accent: "var(--brand)" },
);

const testing = ref(false);

const labels: Record<string, string> = {
  qb_url: "qBittorrent WebUI 地址",
  qb_username: "qB 用户名",
  qb_password: "qB 密码",
  qb_save_path: "qB 默认保存路径",
  qb_category: "qB 默认分类",
};

const placeholders: Record<string, string> = {
  qb_url: "例如 http://192.168.1.10:8080",
  qb_username: "无鉴权可留空",
  qb_password: "",
  qb_save_path: "/downloads（留空使用 qB 默认）",
  qb_category: "留空不分类",
};

function displayLabel(key: string): string {
  return labels[key] ?? key;
}

async function handleTest() {
  const url = (props.form["qb_url"] ?? "").trim();
  if (!url) {
    toast.warning("请先填写 qB WebUI 地址");
    return;
  }
  testing.value = true;
  try {
    await testQB(url, props.form["qb_username"] ?? "", props.form["qb_password"] ?? "");
    toast.success("qB 连接成功");
  } catch (e) {
    toast.error(getApiErrorMessage(e, "qB 连接失败，请检查地址、账号与网络"));
  } finally {
    testing.value = false;
  }
}

defineExpose({ QB_KEYS });
</script>

<template>
  <SettingsCard title="qBittorrent" :accent="accent">
    <template #head-aside>
      <AppButton
        type="button"
        variant="secondary"
        size="sm"
        :disabled="testing"
        @click="handleTest"
      >
        {{ testing ? "测试中…" : "测试连接" }}
      </AppButton>
    </template>

    <SettingsRow
      v-for="key in QB_KEYS"
      :key="key"
      :show-changed-badge="true"
      :changed="isChanged(key)"
    >
      <template #info>
        <div class="settings-row__label">
          <span>{{ displayLabel(key) }}</span>
          <SettingsHelpTooltip
            v-if="key === 'qb_url'"
            title="qBittorrent 地址说明"
          >
            <p>本地 qB 的 WebUI 根地址，例如 <strong>http://192.168.1.10:8080</strong>。</p>
            <p>Docker 部署时不要填 <strong>127.0.0.1</strong>，请填宿主机局域网 IP 或 <strong>http://host.docker.internal:8080</strong>。</p>
            <p>留空则不启用一键下载到 qB。</p>
          </SettingsHelpTooltip>
          <SettingsHelpTooltip
            v-else-if="key === 'qb_username'"
            title="qB 用户名说明"
          >
            <p>qB WebUI 登录用户名，无鉴权可留空。</p>
          </SettingsHelpTooltip>
          <SettingsHelpTooltip
            v-else-if="key === 'qb_password'"
            title="qB 密码说明"
          >
            <p>qB WebUI 登录密码。保存后返回会脱敏显示为 ******。</p>
          </SettingsHelpTooltip>
          <SettingsHelpTooltip
            v-else-if="key === 'qb_save_path'"
            title="默认保存路径说明"
          >
            <p>推送磁力时默认保存目录，留空使用 qB 默认。例：<strong>/downloads</strong></p>
          </SettingsHelpTooltip>
          <SettingsHelpTooltip
            v-else-if="key === 'qb_category'"
            title="默认分类说明"
          >
            <p>推送磁力时默认分类，留空不分类。</p>
          </SettingsHelpTooltip>
        </div>
      </template>
      <template #control>
        <div class="field-text">
          <AppInput
            :model-value="form[key] ?? ''"
            :type="key === 'qb_password' ? 'password' : 'text'"
            :placeholder="placeholders[key] ?? ''"
            autocomplete="off"
            @update:model-value="form[key] = String($event)"
          />
        </div>
      </template>
    </SettingsRow>
  </SettingsCard>
</template>

<style scoped>
.field-text { width: 100%; }
</style>
