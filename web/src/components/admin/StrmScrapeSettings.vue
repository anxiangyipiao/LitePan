<script setup lang="ts">
import { onMounted, ref } from "vue";
import { getApiErrorMessage } from "@/api/client";
import { testMediaOrganizeTmdb } from "@/api/mediaOrganize";
import {
  fetchStrmScrapeSettings,
  saveStrmScrapeSettings,
  testStrmScrape,
  type StrmScrapeSettings,
  type StrmScrapeSource,
  type StrmScrapeWriteMode,
} from "@/api/strmScrape";
import AppButton from "@/components/base/AppButton.vue";
import AppInput from "@/components/base/AppInput.vue";
import AppSelect from "@/components/base/AppSelect.vue";
import AppStateBlock from "@/components/base/AppStateBlock.vue";
import SettingsBoolSegment from "@/components/admin/SettingsBoolSegment.vue";
import SettingsCard from "@/components/admin/SettingsCard.vue";
import SettingsHelpTooltip from "@/components/admin/SettingsHelpTooltip.vue";
import SettingsRow from "@/components/admin/SettingsRow.vue";
import TmdbHostsHelpTip from "@/components/admin/TmdbHostsHelpTip.vue";
import { useSettingsForm, bindSettingsPanelExpose } from "@/composables/useSettingsForm";
import { useSettingsLoad } from "@/composables/useSettingsLoad";
import { toast } from "@/composables/useToast";
import "@/styles/admin-shared.css";

const SCRAPE_SETTINGS_ACCENT = "#38bdf8";

const tmdbLanguageOptions = [
  { value: "zh-CN", label: "简体中文" },
  { value: "zh-TW", label: "繁体中文" },
  { value: "en-US", label: "English" },
];

const writeModeOptions = [
  { value: "missing_only", label: "仅补缺（推荐）" },
  { value: "overwrite", label: "覆盖已有 nfo / 海报" },
];

const sourceOptions = [
  { value: "tmdb", label: "TMDB" },
  { value: "metatube", label: "MetaTube" },
];

const { loading, loaded, runLoad } = useSettingsLoad();
const saving = ref(false);
const testing = ref(false);

const {
  settings,
  isDirty: settingsChanged,
  isFieldChanged,
  snapshotBaseline,
  applyBaseline,
  revert: revertToBaseline,
} = useSettingsForm<StrmScrapeSettings>({
  write_mode: "missing_only",
  source: "tmdb",
  metatube_url: "",
  tmdb_api_key: "",
  tmdb_language: "zh-CN",
  tmdb_request_interval_ms: 300,
  proxy_enabled: false,
  proxy_url: "",
  proxy_username: "",
  proxy_password: "",
});

async function loadSettings(opts?: { silent?: boolean }) {
  await runLoad(
    async () => {
      const data = await fetchStrmScrapeSettings();
      applyBaseline({
        write_mode: (data.write_mode as StrmScrapeWriteMode) || "missing_only",
        source: (data.source as StrmScrapeSource) || "tmdb",
        metatube_url: data.metatube_url || "",
        tmdb_api_key: data.tmdb_api_key || "",
        tmdb_language: data.tmdb_language || "zh-CN",
        tmdb_request_interval_ms: Number(data.tmdb_request_interval_ms) || 300,
        proxy_enabled: Boolean(data.proxy_enabled),
        proxy_url: data.proxy_url || "",
        proxy_username: data.proxy_username || "",
        proxy_password: "",
      });
    },
    "加载 STRM 刮削设置失败",
    { silent: opts?.silent },
  );
}

async function saveSettings() {
  if (saving.value) return;
  saving.value = true;
  try {
    const data = await saveStrmScrapeSettings({
      ...settings,
      tmdb_request_interval_ms: Number(settings.tmdb_request_interval_ms),
    });
    applyBaseline({
      ...settings,
      write_mode: (data.write_mode as StrmScrapeWriteMode) || settings.write_mode,
      source: (data.source as StrmScrapeSource) || settings.source,
      metatube_url: data.metatube_url || settings.metatube_url,
      tmdb_api_key: data.tmdb_api_key || settings.tmdb_api_key,
      proxy_password: "",
    });
    snapshotBaseline();
    toast.success("刮削设置已保存");
  } catch (e) {
    toast.error(getApiErrorMessage(e, "保存失败"));
  } finally {
    saving.value = false;
  }
}

async function testProvider() {
  if (testing.value) return;
  testing.value = true;
  try {
    if (settings.source === "metatube") {
      const url = String(settings.metatube_url || "").trim();
      if (!url) {
        toast.error("请先填写 MetaTube API 地址");
        return;
      }
      const result = await testStrmScrape({ metatube_url: url });
      if (result.ok) {
        toast.success("MetaTube 连通正常");
      } else {
        toast.error("MetaTube 测试未通过");
      }
    } else {
      const result = await testMediaOrganizeTmdb({
        tmdb_api_key: settings.tmdb_api_key,
        tmdb_language: settings.tmdb_language,
        proxy_enabled: settings.proxy_enabled,
        proxy_url: settings.proxy_url,
        proxy_username: settings.proxy_username,
        proxy_password: settings.proxy_password,
        tmdb_request_interval_ms: Number(settings.tmdb_request_interval_ms),
      });
      if (result.ok) {
        toast.success(result.proxy_used ? "TMDB 连通（已走代理）" : "TMDB 连通正常");
      } else {
        toast.error("TMDB 测试未通过");
      }
    }
  } catch (e) {
    toast.error(getApiErrorMessage(e, settings.source === "metatube" ? "MetaTube 测试失败" : "TMDB 测试失败"));
  } finally {
    testing.value = false;
  }
}

onMounted(() => {
  void loadSettings();
});

defineExpose(
  bindSettingsPanelExpose({
    isDirty: settingsChanged,
    saving,
    save: saveSettings,
    reload: () => loadSettings({ silent: loaded.value }),
    revert: revertToBaseline,
  }),
);
</script>

<template>
  <div class="settings-panel" :style="{ '--settings-accent': SCRAPE_SETTINGS_ACCENT }">
    <AppStateBlock v-if="loading && !loaded" message="加载中…" loading min-height="120px" />
    <template v-else>
      <SettingsCard title="刮削策略" :accent="SCRAPE_SETTINGS_ACCENT">
        <SettingsRow :show-changed-badge="true" :changed="isFieldChanged('write_mode')">
          <template #info>
            <div class="settings-row__label">写入策略</div>
          </template>
          <template #control>
            <AppSelect v-model="settings.write_mode" :options="writeModeOptions" />
          </template>
        </SettingsRow>
      </SettingsCard>

      <SettingsCard title="数据源" :accent="SCRAPE_SETTINGS_ACCENT">
        <SettingsRow :show-changed-badge="true" :changed="isFieldChanged('source')">
          <template #info>
            <div class="settings-row__label">刮削数据源</div>
          </template>
          <template #control>
            <AppSelect v-model="settings.source" :options="sourceOptions" />
          </template>
        </SettingsRow>
        <template v-if="settings.source === 'metatube'">
          <SettingsRow :show-changed-badge="true" :changed="isFieldChanged('metatube_url')">
            <template #info>
              <div class="settings-row__label">
                <span>MetaTube API 地址</span>
                <SettingsHelpTooltip title="MetaTube API 地址">
                  <p>自托管 MetaTube 服务的地址（无需 TMDB API Key），例如 https://your-metatube.example.com/。</p>
                  <p>默认地址为示例实例，请替换为你自己的部署。</p>
                </SettingsHelpTooltip>
              </div>
            </template>
            <template #control>
              <AppInput v-model="settings.metatube_url" placeholder="https://your-metatube.example.com/" autocomplete="off" />
            </template>
          </SettingsRow>
          <SettingsRow>
            <template #info>
              <div class="settings-row__label">测试连通性</div>
            </template>
            <template #control>
              <AppButton type="button" variant="secondary" size="sm" :disabled="testing" @click="testProvider">
                {{ testing ? "测试中…" : "测试 MetaTube" }}
              </AppButton>
            </template>
          </SettingsRow>
        </template>
      </SettingsCard>

      <template v-if="settings.source === 'tmdb'">
        <SettingsCard title="TMDB 设置" :accent="SCRAPE_SETTINGS_ACCENT">
          <template #head-aside>
            <p class="scrape-settings-tip">与「目录整理」共用同一套 TMDB / 代理配置，修改后两边同步生效。</p>
          </template>
          <template #head-actions>
            <AppButton type="button" variant="secondary" size="sm" :disabled="testing" @click="testProvider">
              {{ testing ? "测试中…" : "测试连通性" }}
            </AppButton>
          </template>
          <SettingsRow :show-changed-badge="true" :changed="isFieldChanged('tmdb_api_key')">
            <template #info>
              <div class="settings-row__label">TMDB API Key</div>
            </template>
            <template #control>
              <AppInput v-model="settings.tmdb_api_key" type="password" placeholder="请填写 TMDB API Key（必填）" autocomplete="off" />
            </template>
          </SettingsRow>
          <SettingsRow :show-changed-badge="true" :changed="isFieldChanged('tmdb_language')">
            <template #info>
              <div class="settings-row__label">搜索语言</div>
            </template>
            <template #control>
              <AppSelect v-model="settings.tmdb_language" :options="tmdbLanguageOptions" />
            </template>
          </SettingsRow>
          <SettingsRow :show-changed-badge="true" :changed="isFieldChanged('tmdb_request_interval_ms')">
            <template #info>
              <div class="settings-row__label">
                <span>请求间隔（毫秒）</span>
                <SettingsHelpTooltip title="请求间隔说明">
                  <p>连续请求 TMDB 的最小间隔，过小可能触发限流。</p>
                </SettingsHelpTooltip>
              </div>
            </template>
            <template #control>
              <AppInput v-model="settings.tmdb_request_interval_ms" type="number" min="200" max="5000" />
            </template>
          </SettingsRow>
        </SettingsCard>

        <SettingsCard title="代理设置" :accent="SCRAPE_SETTINGS_ACCENT">
          <template #head-aside>
            <TmdbHostsHelpTip />
          </template>
          <SettingsRow :show-changed-badge="true" :changed="isFieldChanged('proxy_enabled')">
            <template #info>
              <div class="settings-row__label">启用代理</div>
            </template>
            <template #control>
              <SettingsBoolSegment v-model="settings.proxy_enabled" label="启用代理访问 TMDB" />
            </template>
          </SettingsRow>
          <template v-if="settings.proxy_enabled">
            <SettingsRow :show-changed-badge="true" :changed="isFieldChanged('proxy_url')">
              <template #info>
                <div class="settings-row__label">代理地址</div>
              </template>
              <template #control>
                <AppInput v-model="settings.proxy_url" placeholder="http://127.0.0.1:1080 或 socks5://127.0.0.1:1080" />
              </template>
            </SettingsRow>
            <SettingsRow :show-changed-badge="true" :changed="isFieldChanged('proxy_username')">
              <template #info>
                <div class="settings-row__label">用户名</div>
              </template>
              <template #control>
                <AppInput v-model="settings.proxy_username" placeholder="可选" autocomplete="off" />
              </template>
            </SettingsRow>
            <SettingsRow :show-changed-badge="true" :changed="isFieldChanged('proxy_password')">
              <template #info>
                <div class="settings-row__label">密码</div>
              </template>
              <template #control>
                <AppInput v-model="settings.proxy_password" type="password" placeholder="不修改请留空" autocomplete="off" />
              </template>
            </SettingsRow>
          </template>
        </SettingsCard>
      </template>
    </template>
  </div>
</template>

<style scoped>
.scrape-settings-tip {
  margin: 0;
  padding: 0;
  color: var(--text-muted);
  font-size: 12px;
  line-height: 1.45;
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}
.settings-row__label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-weight: 600;
  color: var(--text);
}
</style>
