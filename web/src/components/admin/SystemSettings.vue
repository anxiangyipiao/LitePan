<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { getApiErrorMessage } from "@/api/client";
import {
  fetchSystemConfig,
  updateCredentials,
} from "@/api/auth";
import {
  fetchSettings,
  saveSettings,
  type SettingCategory,
  type SettingItem,
} from "@/api/settings";
import { toast } from "@/composables/useToast";
import { useSettingsLoad } from "@/composables/useSettingsLoad";
import { useAdminPageLoading } from "@/composables/useAdminLoadingBar";
import { useSectionTabRoute } from "@/composables/useSectionTabRoute";
import { useSettingsPageDirty } from "@/composables/useSettingsPageDirty";
import { useAuthStore } from "@/stores/auth";
import AppButton from "@/components/base/AppButton.vue";
import AppInput from "@/components/base/AppInput.vue";
import AppSelect from "@/components/base/AppSelect.vue";
import SettingsBoolSegment from "@/components/admin/SettingsBoolSegment.vue";
import SectionTabBar from "@/components/admin/SectionTabBar.vue";
import SettingsCard from "@/components/admin/SettingsCard.vue";
import SettingsRow from "@/components/admin/SettingsRow.vue";
import SettingsHelpTooltip from "@/components/admin/SettingsHelpTooltip.vue";
import ApiKeySettings from "@/components/admin/ApiKeySettings.vue";
import { isCacheSettingKey } from "@/constants/cacheSettings";
import "@/styles/admin-shared.css";

const props = withDefaults(
  defineProps<{
    forcePasswordChange?: boolean;
    passwordChangeReason?: string;
  }>(),
  { forcePasswordChange: false, passwordChangeReason: "" },
);

const emit = defineEmits<{ "password-updated": []; "admin-ui-updated": [] }>();

const SECURITY_TAB = "security";
const HOMEPAGE_TAB = "homepage";
const SERVICE_TAB = "services";
const API_KEYS_TAB = "apiKeys";

const ACCENTS = ["var(--brand)", "#f59e0b", "#10b981", "#6366f1", "#ec4899"];

const auth = useAuthStore();

const { loading, runLoad } = useSettingsLoad();
useAdminPageLoading("settings", loading);
const saving = ref(false);
const categories = ref<SettingCategory[]>([]);
const items = ref<SettingItem[]>([]);
const settingsLoaded = ref(false);
const form = reactive<Record<string, string>>({});
const original = reactive<Record<string, string>>({});

const newPassword = ref("");
const confirmPassword = ref("");
const securityForm = reactive({
  admin_username: "admin",
  session_timeout: "2",
});
const securityOriginal = reactive({
  admin_username: "admin",
  session_timeout: "2",
});
const homepageForm = reactive({
  public_index_enabled: true,
  index_strm_auto_detect_enabled: true,
});
const homepageOriginal = reactive({
  public_index_enabled: true,
  index_strm_auto_detect_enabled: true,
});
const apiKeySettingsRef = ref<InstanceType<typeof ApiKeySettings> | null>(null);
const apiKeyToolbar = reactive({ loading: true, keyCount: 0, maxKeys: 10 });
const apiKeyAddDisabled = computed(
  () => apiKeyToolbar.loading || apiKeyToolbar.keyCount >= apiKeyToolbar.maxKeys,
);

const tabs = computed(() => [
  { key: SECURITY_TAB, label: "账号安全" },
  { key: HOMEPAGE_TAB, label: "首页设置" },
  { key: SERVICE_TAB, label: "其他设置", disabled: props.forcePasswordChange },
  { key: API_KEYS_TAB, label: "API 秘钥", disabled: props.forcePasswordChange },
]);

const systemItems = computed(() => items.value.filter((it) => it.category === "system"));
const systemChangedKeys = computed(() => systemItems.value.filter((it) => isChanged(it)).map((it) => it.key));
const systemChangedCount = computed(() => systemChangedKeys.value.length);

const passwordsMismatch = computed(
  () =>
    Boolean(newPassword.value && confirmPassword.value) &&
    newPassword.value !== confirmPassword.value,
);

const securityDirty = computed(
  () =>
    securityForm.admin_username !== securityOriginal.admin_username ||
    securityForm.session_timeout !== securityOriginal.session_timeout ||
    newPassword.value !== "" ||
    confirmPassword.value !== "",
);

const homepageDirty = computed(
  () =>
    homepageForm.public_index_enabled !== homepageOriginal.public_index_enabled ||
    homepageForm.index_strm_auto_detect_enabled !== homepageOriginal.index_strm_auto_detect_enabled,
);

const servicesDirty = computed(() => systemChangedCount.value > 0);

function revertSecurityDraft() {
  securityForm.admin_username = securityOriginal.admin_username;
  securityForm.session_timeout = securityOriginal.session_timeout;
  newPassword.value = "";
  confirmPassword.value = "";
}

function revertHomepageDraft() {
  Object.assign(homepageForm, homepageOriginal);
}

function revertServicesDraft() {
  for (const it of systemItems.value) form[it.key] = original[it.key];
}

function isTabDirty(tab: string): boolean {
  if (tab === SECURITY_TAB) return securityDirty.value;
  if (tab === HOMEPAGE_TAB) return homepageDirty.value;
  if (tab === SERVICE_TAB) return servicesDirty.value;
  return false;
}

function revertCurrentTab(tab = activeTab.value) {
  if (tab === SECURITY_TAB) revertSecurityDraft();
  else if (tab === HOMEPAGE_TAB) revertHomepageDraft();
  else if (tab === SERVICE_TAB) revertServicesDraft();
}

const settingsDirty = computed(
  () => securityDirty.value || homepageDirty.value || servicesDirty.value,
);
const { activeTab, setActiveTab } = useSectionTabRoute(
  SECURITY_TAB,
  [SECURITY_TAB, HOMEPAGE_TAB, SERVICE_TAB, API_KEYS_TAB],
  {
    beforeTabChange: async (from, to) => {
      if (props.forcePasswordChange && to !== SECURITY_TAB) return false;
      if (!isTabDirty(from)) return true;
      return confirmDiscardChanges(() => isTabDirty(from));
    },
  },
);
const { confirmDiscardChanges } = useSettingsPageDirty(settingsDirty, revertCurrentTab);

const isSecurityTab = computed(() => activeTab.value === SECURITY_TAB);
const isHomepageTab = computed(() => activeTab.value === HOMEPAGE_TAB);
const isServicesTab = computed(() => activeTab.value === SERVICE_TAB);
const isApiKeysTab = computed(() => activeTab.value === API_KEYS_TAB);
const accentColor = computed(() => {
  if (isSecurityTab.value) return ACCENTS[0];
  if (isHomepageTab.value) return ACCENTS[1];
  if (isServicesTab.value) return ACCENTS[2];
  if (isApiKeysTab.value) return ACCENTS[3];
  return ACCENTS[0];
});

const canSave = computed(() => {
  if (isSecurityTab.value) return securityDirty.value && !passwordsMismatch.value;
  if (isHomepageTab.value) return homepageDirty.value;
  if (isServicesTab.value) return servicesDirty.value;
  return false;
});

function isChanged(it: SettingItem): boolean {
  return form[it.key] !== original[it.key];
}

function displayLabel(it: SettingItem): string {
  if (it.type === "int" && it.unit) return `${it.label}（${it.unit}）`;
  return it.label;
}

function filterOutCacheSettings(payload: {
  categories: SettingCategory[];
  items: SettingItem[];
}) {
  const visibleItems = (payload.items ?? []).filter(
    (it) => !isCacheSettingKey(it.key) && it.key !== "upload_task_concurrency",
  );
  const visibleCatIds = new Set(visibleItems.map((it) => it.category));
  const visibleCategories = (payload.categories ?? []).filter((c) => visibleCatIds.has(c.id));
  return { categories: visibleCategories, items: visibleItems };
}

function applyPayload(payload: { categories: SettingCategory[]; items: SettingItem[] }) {
  const filtered = filterOutCacheSettings(payload);
  categories.value = filtered.categories;
  items.value = filtered.items;
  settingsLoaded.value = true;
  for (const it of items.value) {
    form[it.key] = it.value;
    original[it.key] = it.value;
  }
}

function applySystemConfig(config: {
  admin_username: string;
  session_timeout: number;
  public_index_enabled: boolean;
  index_strm_auto_detect_enabled?: boolean;
}) {
  securityForm.admin_username = config.admin_username || "admin";
  securityForm.session_timeout = String(config.session_timeout || 2);
  securityOriginal.admin_username = securityForm.admin_username;
  securityOriginal.session_timeout = securityForm.session_timeout;
  homepageForm.public_index_enabled = config.public_index_enabled ?? true;
  homepageOriginal.public_index_enabled = homepageForm.public_index_enabled;
  homepageForm.index_strm_auto_detect_enabled = config.index_strm_auto_detect_enabled ?? true;
  homepageOriginal.index_strm_auto_detect_enabled = homepageForm.index_strm_auto_detect_enabled;
}

async function loadSystemConfig() {
  const config = await fetchSystemConfig();
  applySystemConfig(config);
  if (props.forcePasswordChange || config.must_change_password) {
    activeTab.value = SECURITY_TAB;
  }
}

async function loadSettings() {
  applyPayload(await fetchSettings());
}

async function load() {
  await runLoad(async () => {
    if (props.forcePasswordChange) {
      await loadSystemConfig();
    } else {
      await Promise.all([loadSettings(), loadSystemConfig()]);
    }
  }, "加载设置失败");
}

onMounted(load);

watch(
  () => props.forcePasswordChange,
  async (locked, prevLocked) => {
    if (locked) {
      activeTab.value = SECURITY_TAB;
      return;
    }
    if (prevLocked && !settingsLoaded.value) {
      await runLoad(async () => {
        await loadSettings();
      }, "加载设置失败");
    }
  },
);

async function saveSecurity() {
  if (props.forcePasswordChange && !newPassword.value) {
    toast.error("当前管理员密码需要升级，请先设置新密码");
    return;
  }
  if (newPassword.value && !confirmPassword.value) {
    toast.error("请再次输入新密码进行确认");
    return;
  }
  if (passwordsMismatch.value) {
    toast.error("两次输入的密码不一致");
    return;
  }
  const sessionTimeout = Number.parseFloat(securityForm.session_timeout);
  if (!Number.isFinite(sessionTimeout) || sessionTimeout < 0.5 || sessionTimeout > 24) {
    toast.error("会话超时时间必须在 0.5-24 小时之间");
    return;
  }

  saving.value = true;
  try {
    await updateCredentials({
      admin_username: securityForm.admin_username.trim(),
      admin_password: newPassword.value || undefined,
      session_timeout: sessionTimeout,
    });
    const passwordUpdated = Boolean(newPassword.value);
    newPassword.value = "";
    confirmPassword.value = "";
    toast.success("账号与安全设置已保存");
    await loadSystemConfig();
    if (passwordUpdated) emit("password-updated");
  } catch (e) {
    toast.error(getApiErrorMessage(e, "保存失败"));
  } finally {
    saving.value = false;
  }
}

async function saveHomepage() {
  saving.value = true;
  try {
    await updateCredentials({
      admin_username: securityForm.admin_username.trim(),
      public_index_enabled: homepageForm.public_index_enabled,
      index_strm_auto_detect_enabled: homepageForm.index_strm_auto_detect_enabled,
    });
    toast.success("首页设置已保存");
    await loadSystemConfig();
    await auth.load();
    emit("admin-ui-updated");
  } catch (e) {
    toast.error(getApiErrorMessage(e, "保存失败"));
  } finally {
    saving.value = false;
  }
}

async function saveServices() {
  if (!servicesDirty.value) return;
  saving.value = true;
  try {
    const changed: Record<string, string> = {};
    for (const key of systemChangedKeys.value) changed[key] = form[key];
    if (Object.keys(changed).length > 0) {
      applyPayload(await saveSettings(changed));
    }
    toast.success("其他设置已保存");
  } catch (e) {
    toast.error(getApiErrorMessage(e, "保存失败"));
  } finally {
    saving.value = false;
  }
}

async function submit() {
  if (isSecurityTab.value) {
    await saveSecurity();
    return;
  }
  if (isHomepageTab.value) {
    await saveHomepage();
    return;
  }
  if (isServicesTab.value) {
    await saveServices();
  }
}
</script>

<template>
  <div class="settings">
    <SectionTabBar :model-value="activeTab" :tabs="tabs" @update:model-value="setActiveTab">
      <template #actions>
        <AppButton
          v-if="isApiKeysTab"
          type="button"
          variant="primary"
          :disabled="apiKeyAddDisabled"
          @click="apiKeySettingsRef?.openCreate()"
        >
          新增秘钥
        </AppButton>
        <AppButton
          v-else
          type="button"
          variant="primary"
          :disabled="!canSave || saving"
          @click="submit"
        >
          {{ saving ? "保存中…" : "保存改动" }}
        </AppButton>
      </template>
    </SectionTabBar>

    <template v-if="!loading">
      <SettingsCard v-if="isSecurityTab" title="账号安全" :accent="accentColor">
        <SettingsRow
          :show-changed-badge="true"
          :changed="securityForm.admin_username !== securityOriginal.admin_username"
        >
          <template #info>
            <div class="settings-row__label">
              <span>管理员用户名</span>
            </div>
          </template>
          <template #control>
            <div class="field-text">
              <AppInput
                v-model="securityForm.admin_username"
                placeholder="admin"
                autocomplete="username"
              />
            </div>
          </template>
        </SettingsRow>

        <SettingsRow :show-changed-badge="true" :changed="Boolean(newPassword)">
          <template #info>
            <div class="settings-row__label">
              <span>新密码</span>
            </div>
          </template>
          <template #control>
            <div class="field-text">
              <AppInput
                v-model="newPassword"
                type="password"
                placeholder="留空表示不修改"
                autocomplete="new-password"
                ignore-autofill
              />
            </div>
          </template>
        </SettingsRow>

        <SettingsRow :show-changed-badge="true" :changed="Boolean(newPassword)">
          <template #info>
            <div class="settings-row__label">
              <span>确认新密码</span>
            </div>
          </template>
          <template #control>
            <div class="field-text">
              <AppInput
                v-model="confirmPassword"
                type="password"
                placeholder="再次输入新密码"
                autocomplete="new-password"
                ignore-autofill
              />
              <p v-if="passwordsMismatch" class="field-error">两次输入的密码不一致</p>
            </div>
          </template>
        </SettingsRow>

        <SettingsRow
          :show-changed-badge="true"
          :changed="securityForm.session_timeout !== securityOriginal.session_timeout"
        >
          <template #info>
            <div class="settings-row__label">
              <span>会话超时时间（小时）</span>
            </div>
          </template>
          <template #control>
            <div class="field-num">
              <AppInput v-model="securityForm.session_timeout" type="number" placeholder="2" />
            </div>
          </template>
        </SettingsRow>
      </SettingsCard>

      <SettingsCard v-else-if="isHomepageTab" title="访问权限" :accent="accentColor">
        <SettingsRow
          :show-changed-badge="true"
          :changed="homepageForm.public_index_enabled !== homepageOriginal.public_index_enabled"
        >
          <template #info>
            <div class="settings-row__label">
              <span>允许匿名访问文件列表</span>
              <SettingsHelpTooltip title="匿名文件列表访问说明">
                <p>开启后，访客无需登录即可访问首页并浏览文件列表。</p>
                <p>关闭后，未登录用户访问首页会自动跳转到登录页，匿名公开接口也会同时拒绝访问。</p>
              </SettingsHelpTooltip>
            </div>
          </template>
          <template #control>
            <SettingsBoolSegment
              v-model="homepageForm.public_index_enabled"
              label="允许匿名访问文件列表"
              off-label="不允许"
              on-label="允许"
            />
          </template>
        </SettingsRow>
      </SettingsCard>

      <SettingsCard v-if="isHomepageTab" title="STRM" :accent="accentColor">
        <SettingsRow
          :show-changed-badge="true"
          :changed="homepageForm.index_strm_auto_detect_enabled !== homepageOriginal.index_strm_auto_detect_enabled"
        >
          <template #info>
            <div class="settings-row__label">
              <span>自动检测 STRM 变化</span>
              <SettingsHelpTooltip title="自动检测 STRM 变化说明">
                <p>开启后，管理员进入已配置 STRM 任务的目录时，会自动检测本地未生成的 STRM 与元数据，并在文件列表上方提示是否立即生成。</p>
                <p>关闭后，进入目录不再发起检测请求，可节省开销；如需同步，请依赖 STRM 定时任务或在 STRM 管理页手动触发扫描。</p>
              </SettingsHelpTooltip>
            </div>
          </template>
          <template #control>
            <SettingsBoolSegment
              v-model="homepageForm.index_strm_auto_detect_enabled"
              label="自动检测 STRM 变化"
              off-label="关闭"
              on-label="打开"
            />
          </template>
        </SettingsRow>
      </SettingsCard>


      <template v-else-if="isServicesTab">
        <SettingsCard v-if="systemItems.length" title="授权与日志" :accent="accentColor">
          <SettingsRow
            v-for="it in systemItems"
            :key="it.key"
            :show-changed-badge="true"
            :changed="isChanged(it)"
          >
            <template #info>
              <div class="settings-row__label">
                <span>{{ displayLabel(it) }}</span>
                <SettingsHelpTooltip
                  v-if="it.key === 'oauth_server_url'"
                  title="OAuth 代理服务地址说明"
                >
                  <p>用于主程序对接 OAuth 认证代理服务，会影响相关驱动的授权、刷新和回调地址处理。</p>
                  <p>添加账号时「自动获取 Token」经此服务转发。留空或无效地址将回落默认值。</p>
                  <p>示例：<strong>https://oauth.litepan.top</strong></p>
                </SettingsHelpTooltip>
                <SettingsHelpTooltip
                  v-else-if="it.key === 'auth_active_refresh_enabled'"
                  title="主动认证刷新说明"
                >
                  <p>程序根据各网盘过期时间提前刷新 token，减少访问时遇到认证过期的概率。</p>
                  <div class="settings-help__section">推荐开启</div>
                  <div class="settings-help__item">
                    <span class="settings-help__dot settings-help__dot--on" />
                    <span>NAS / Docker / 服务器 24 小时常驻：token 始终健康，缓存、STRM、反代等后台任务更稳定</span>
                  </div>
                  <div class="settings-help__section">推荐关闭</div>
                  <div class="settings-help__item">
                    <span class="settings-help__dot settings-help__dot--off" />
                    <span>桌面端临时使用、用完即关：无需后台维护 token，访问时被动刷新即可</span>
                  </div>
                  <div class="settings-help__item">
                    <span class="settings-help__dot settings-help__dot--off" />
                    <span>同一账号还在其他挂载工具或脚本里使用：避免多个程序争抢 refresh_token，尤其是 115 网盘</span>
                  </div>
                </SettingsHelpTooltip>
                <SettingsHelpTooltip
                  v-else-if="it.description"
                  :title="`${displayLabel(it)}说明`"
                >
                  <p>{{ it.description }}</p>
                </SettingsHelpTooltip>
              </div>
            </template>
            <template #control>
              <div class="settings-row__field">
                <SettingsBoolSegment
                  v-if="it.type === 'bool'"
                  :model-value="form[it.key] === 'true'"
                  :label="displayLabel(it)"
                  @update:model-value="form[it.key] = $event ? 'true' : 'false'"
                />

                <div v-else-if="it.type === 'select'" class="field-select">
                  <AppSelect
                    :model-value="form[it.key]"
                    :options="it.options || []"
                    @update:model-value="form[it.key] = String($event)"
                  />
                </div>

                <div v-else-if="it.type === 'int'" class="field-num">
                  <AppInput v-model="form[it.key]" type="number" :placeholder="it.default" />
                </div>

                <div v-else class="field-text">
                  <AppInput v-model="form[it.key]" :placeholder="it.default" autocomplete="off" />
                </div>
              </div>
            </template>
          </SettingsRow>
        </SettingsCard>
      </template>

      <ApiKeySettings
        v-else-if="isApiKeysTab"
        ref="apiKeySettingsRef"
        :accent="accentColor"
        @toolbar-state="Object.assign(apiKeyToolbar, $event)"
      />
    </template>
  </div>
</template>

<style scoped>
.settings {
  padding-bottom: 24px;
}

.field-error {
  margin: 6px 0 0;
  font-size: 12px;
  color: #dc2626;
  text-align: right;
}

.field-text,
.field-num,
.field-select {
  width: 100%;
}
</style>
