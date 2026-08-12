<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { storeToRefs } from "pinia";
import { useAccountsStore } from "@/stores/accounts";
import { getApiErrorMessage } from "@/api/client";
import type { Account, FieldSchema } from "@/api/types";
import { toast } from "@/composables/useToast";
import { useOAuthAuth } from "@/composables/useOAuthAuth";
import AppModal from "@/components/base/AppModal.vue";
import AppButton from "@/components/base/AppButton.vue";
import AppInput from "@/components/base/AppInput.vue";
import FormField from "@/components/base/FormField.vue";
import DriverPickerStep from "./DriverPickerStep.vue";
import QrLoginModal from "./QrLoginModal.vue";
import DynamicForm from "@/components/form/DynamicForm.vue";

const props = defineProps<{ open: boolean; editing: Account | null }>();
const emit = defineEmits<{ close: []; saved: [] }>();

const store = useAccountsStore();
const { drivers, accounts } = storeToRefs(store);
const { loading: oauthLoading, run: runOAuth, cancel: cancelOAuth } = useOAuthAuth();

const step = ref<1 | 2>(1);
const driverType = ref("");
const name = ref("");
const formValues = ref<Record<string, unknown>>({});
const submitting = ref(false);
// 账号级自定义盘条图标：图片 URL 或上传后的 data URL，存进 config 的保留键 _card_logo。
const cardLogo = ref("");
const logoFileInput = ref<HTMLInputElement | null>(null);

const selectedDriver = computed(() => store.driverOf(driverType.value));
const fields = computed<FieldSchema[]>(() => selectedDriver.value?.fields ?? []);
const isEdit = computed(() => props.editing !== null);
const stepTitle = computed(() =>
  isEdit.value ? "编辑账号" : step.value === 1 ? "选择网盘驱动" : "配置账号信息",
);
const supportsOAuth = computed(() => Boolean(selectedDriver.value?.supports_oauth));
const supportsQRLogin = computed(() => Boolean(selectedDriver.value?.supports_qr_login));
const qrOpen = ref(false);

function parseBooleanSelectValue(f: FieldSchema, raw: unknown) {
  if (f.type !== "select" || !(f.options ?? []).some((o) => o.value === "true" || o.value === "false")) {
    return raw;
  }
  if (typeof raw === "boolean") return raw;
  const normalized = String(raw ?? "").trim().toLowerCase();
  if (normalized === "true") return true;
  if (normalized === "false") return false;
  return raw;
}

function fieldInitialValue(f: FieldSchema, preset: Record<string, unknown>) {
  const raw = preset[f.name] ?? (f.type === "bool" ? false : (f.default ?? ""));
  return parseBooleanSelectValue(f, raw);
}

function initForm(fs: FieldSchema[], preset: Record<string, unknown> = {}) {
  const v: Record<string, unknown> = { ...preset };
  for (const f of fs) {
    v[f.name] = fieldInitialValue(f, preset);
  }
  formValues.value = v;
}

function resetDialog() {
  cancelOAuth();
  step.value = 1;
  driverType.value = "";
  name.value = "";
  formValues.value = {};
  cardLogo.value = "";
}

watch(
  () => props.open,
  (open) => {
    if (!open) {
      cancelOAuth();
      return;
    }
    store.loadDrivers();
    store.loadAccounts();
    if (props.editing) {
      driverType.value = props.editing.driver_type;
      name.value = props.editing.name;
      let preset: Record<string, unknown> = {};
      try {
        preset = JSON.parse(props.editing.config || "{}");
      } catch {
        preset = {};
      }
      initForm(store.driverOf(props.editing.driver_type)?.fields ?? [], preset);
      cardLogo.value = typeof preset._card_logo === "string" ? preset._card_logo : "";
      step.value = 2;
    } else {
      resetDialog();
    }
  },
);

function goPrevStep() {
  cancelOAuth();
  formValues.value = {};
  step.value = 1;
}

function goStep2() {
  if (!driverType.value) {
    toast.warning("请选择驱动类型");
    return;
  }
  initForm(fields.value);
  step.value = 2;
}

function buildConfig(): string {
  const cfg: Record<string, unknown> = {};
  const schemaNames = new Set(fields.value.map((f) => f.name));
  for (const f of fields.value) {
    const raw = formValues.value[f.name];
    if (f.type === "bool") {
      cfg[f.name] = Boolean(raw);
    } else if (f.type === "number") {
      const s = String(raw ?? "").trim();
      if (s) cfg[f.name] = s;
    } else if (f.type === "select") {
      const normalized = parseBooleanSelectValue(f, raw);
      if (typeof normalized === "boolean") {
        cfg[f.name] = normalized;
        continue;
      }
      const s = String(normalized ?? "").trim();
      if (s) cfg[f.name] = s;
      else if (f.default) cfg[f.name] = parseBooleanSelectValue(f, f.default);
    } else {
      const s = String(raw ?? "").trim();
      if (s) cfg[f.name] = s;
      else if (f.default) cfg[f.name] = f.default;
    }
  }
  for (const [key, raw] of Object.entries(formValues.value)) {
    if (schemaNames.has(key)) continue;
    const s = String(raw ?? "").trim();
    if (s) cfg[key] = s;
  }
  // 空值也写入，覆盖层读到空字符串即回退驱动默认图标
  cfg["_card_logo"] = cardLogo.value.trim();
  return JSON.stringify(cfg);
}

function validate(): string | null {
  const trimmed = name.value.trim();
  if (!trimmed) return "请填写账号名称";
  const dup = accounts.value.find(
    (a) =>
      a.name.toLowerCase() === trimmed.toLowerCase() &&
      (!props.editing || a.id !== props.editing.id),
  );
  if (dup) return "账号名称已存在，请使用其他名称";
  for (const f of fields.value) {
    if (!f.required) continue;
    const v = formValues.value[f.name];
    if (f.type === "bool") continue;
    if (!String(v ?? "").trim() && !f.default) return `请填写「${f.label}」`;
  }
  return null;
}

function openLogoPicker() {
  logoFileInput.value?.click();
}

// 上传图片 → 画布缩到 96px 内 → PNG data URL。盘条徽标只需小图，避免把大图塞进 config。
async function readLogoFile(file: File): Promise<string> {
  if (file.size > 1024 * 1024) throw new Error("图片不能超过 1MB");
  const objectURL = URL.createObjectURL(file);
  try {
    const img = new Image();
    await new Promise<void>((resolve, reject) => {
      img.onload = () => resolve();
      img.onerror = () => reject(new Error("图片解析失败"));
      img.src = objectURL;
    });
    const MAX = 96;
    const scale = Math.min(1, MAX / Math.max(img.width, img.height));
    const w = Math.max(1, Math.round(img.width * scale));
    const h = Math.max(1, Math.round(img.height * scale));
    const canvas = document.createElement("canvas");
    canvas.width = w;
    canvas.height = h;
    const ctx = canvas.getContext("2d");
    if (!ctx) throw new Error("无法处理图片");
    ctx.drawImage(img, 0, 0, w, h);
    return canvas.toDataURL("image/png");
  } finally {
    URL.revokeObjectURL(objectURL);
  }
}

async function onLogoFileChange(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = "";
  if (!file) return;
  try {
    cardLogo.value = await readLogoFile(file);
  } catch (e) {
    toast.error(e instanceof Error ? e.message : "图片读取失败");
  }
}

async function submit() {
  const err = validate();
  if (err) {
    toast.warning(err);
    return;
  }
  submitting.value = true;
  try {
    const payload = {
      name: name.value.trim(),
      driver_type: driverType.value,
      config: buildConfig(),
      is_active: props.editing?.is_active ?? true,
      is_default: props.editing?.is_default ?? false,
      sort_order: props.editing?.sort_order ?? 0,
    };
    if (props.editing) {
      await store.update(props.editing.id, payload);
      toast.success("账号已更新");
    } else {
      await store.create(payload);
      toast.success("账号已添加");
    }
    emit("saved");
    emit("close");
  } catch (e) {
    toast.error(getApiErrorMessage(e, "保存失败"));
  } finally {
    submitting.value = false;
  }
}

async function handleOAuth() {
  if (!driverType.value) return;
  const fieldNames = fields.value.map((f) => f.name);
  try {
    const filled = await runOAuth(driverType.value, fieldNames);
    formValues.value = { ...formValues.value, ...filled };
  } catch {
    /* toast 已在 composable 内处理 */
  }
}

function openQRLogin() {
  if (!driverType.value) return;
  qrOpen.value = true;
}

function onQRSuccess(credentials: Record<string, string>) {
  formValues.value = { ...formValues.value, ...credentials };
  qrOpen.value = false;
}

function handleClose() {
  cancelOAuth();
  qrOpen.value = false;
  emit("close");
}
</script>

<template>
  <AppModal :open="open" size="account" @close="handleClose">
    <template #header>
      <div class="dialog-head">
        <span v-if="!isEdit" class="step-badge">{{ step }}</span>
        <h3 class="dialog-head__title">{{ stepTitle }}</h3>
      </div>
    </template>

    <DriverPickerStep
      v-if="step === 1 && !isEdit"
      v-model="driverType"
      :drivers="drivers"
      @next="goStep2"
    />

    <div v-else class="form">
      <FormField label="账号名称" required>
        <AppInput v-model="name" placeholder="请输入账号名称" />
      </FormField>
      <DynamicForm :fields="fields" v-model="formValues" />
      <FormField label="盘条图标（可选）">
        <div class="card-logo-field">
          <span
            class="card-logo-field__preview"
            :class="{ 'card-logo-field__preview--has': cardLogo }"
          >
            <img v-if="cardLogo" :src="cardLogo" alt="盘条图标" />
            <span v-else class="card-logo-field__ph" aria-hidden="true">图</span>
          </span>
          <div class="card-logo-field__controls">
            <AppInput v-model="cardLogo" placeholder="粘贴图片 URL，或上传一张小图" />
            <div class="card-logo-field__row">
              <AppButton type="button" variant="secondary" size="sm" @click="openLogoPicker">
                上传图片
              </AppButton>
              <AppButton v-if="cardLogo" type="button" variant="ghost" size="sm" @click="cardLogo = ''">
                清除
              </AppButton>
            </div>
          </div>
          <input
            ref="logoFileInput"
            type="file"
            accept="image/*"
            hidden
            @change="onLogoFileChange"
          />
        </div>
      </FormField>
    </div>

    <template v-if="step === 2" #footer>
      <div class="step-footer">
        <div class="step-footer__left">
          <AppButton
            v-if="supportsOAuth"
            variant="primary"
            :disabled="oauthLoading || submitting"
            @click="handleOAuth"
          >
            {{ oauthLoading ? "正在获取…" : isEdit ? "重新获取 Token" : "自动获取 Token" }}
          </AppButton>
          <AppButton
            v-if="supportsQRLogin"
            variant="primary"
            :disabled="submitting"
            @click="openQRLogin"
          >
            {{ isEdit ? "重新扫码获取" : "扫码获取授权" }}
          </AppButton>
        </div>
        <div class="step-footer__right">
          <AppButton v-if="!isEdit" variant="secondary" @click="goPrevStep">← 上一步</AppButton>
          <AppButton variant="primary" :disabled="submitting" @click="submit">
            {{ submitting ? "测试中…" : isEdit ? "保存修改" : "添加账号" }}
          </AppButton>
        </div>
      </div>
    </template>
  </AppModal>

  <QrLoginModal
    :open="qrOpen"
    :driver-type="driverType"
    @success="onQRSuccess"
    @close="qrOpen = false"
  />
</template>

<style scoped>
.dialog-head {
  display: flex;
  align-items: center;
  gap: 12px;
}
.step-badge {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: var(--brand-gradient-h);
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 600;
  font-size: 12px;
  flex-shrink: 0;
}
.dialog-head__title {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: var(--text);
}

.form {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-height: 420px;
  overflow-y: auto;
  padding-right: 4px;
}

.step-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 100%;
  gap: 12px;
}
.step-footer__left {
  flex-shrink: 0;
}
.step-footer__right {
  display: flex;
  gap: 10px;
  margin-left: auto;
}

/* 窄屏：底部按钮区允许换行，避免 OAuth/扫码/上一步/保存 挤出一行溢出 */
@media (max-width: 768px) {
  .step-footer {
    flex-wrap: wrap;
  }
  .step-footer__right {
    flex-wrap: wrap;
    justify-content: flex-end;
  }
}

.card-logo-field {
  display: flex;
  align-items: flex-start;
  gap: 10px;
}
.card-logo-field__preview {
  flex: 0 0 auto;
  width: 40px;
  height: 40px;
  border-radius: 12px;
  display: grid;
  place-items: center;
  overflow: hidden;
  background: var(--surface-sunken);
  border: 1px solid var(--border-soft);
  color: var(--text-muted);
  font-size: 13px;
  font-weight: 700;
}
.card-logo-field__preview--has {
  border-color: var(--brand);
}
.card-logo-field__preview img {
  width: 100%;
  height: 100%;
  object-fit: contain;
  display: block;
  background: #fff;
}
.card-logo-field__controls {
  flex: 1 1 auto;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.card-logo-field__row {
  display: flex;
  align-items: center;
  gap: 8px;
}

:root[data-skin="brutal"] .step-badge {
  border: var(--brutal-bw) solid var(--brutal-ink);
  border-radius: 0;
  background: var(--brand);
  color: var(--text-on-brand);
}
</style>
