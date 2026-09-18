import { useEffect, useMemo, useState } from "react";
import toast from "react-hot-toast";
import { DesignSystem } from "../../types";
import { NEW_DESIGN_SYSTEM_CONTENT } from "../../lib/design-systems";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Textarea } from "../ui/textarea";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../ui/select";
import { useI18n } from "../../i18n";

interface Props {
  designSystems: DesignSystem[];
  selectedDesignSystemId: string | null;
  setSelectedDesignSystemId: (id: string | null) => void;
  initialEditingId?: string | null;
  createDesignSystem: (payload: {
    name: string;
    content: string;
  }) => Promise<DesignSystem>;
  updateDesignSystem: (
    id: string,
    payload: { name?: string; content?: string }
  ) => Promise<DesignSystem>;
  deleteDesignSystem: (id: string) => Promise<void>;
}

function DesignSystemsManager({
  designSystems,
  selectedDesignSystemId,
  setSelectedDesignSystemId,
  initialEditingId,
  createDesignSystem,
  updateDesignSystem,
  deleteDesignSystem,
}: Props) {
  const { t } = useI18n();
  const [editingId, setEditingId] = useState<string | null>(
    initialEditingId ?? null
  );
  const [draftName, setDraftName] = useState("");
  const [draftContent, setDraftContent] = useState("");
  const [isSaving, setIsSaving] = useState(false);

  const editingDesignSystem = useMemo(
    () => designSystems.find((item) => item.id === editingId) ?? null,
    [designSystems, editingId]
  );

  useEffect(() => {
    if (initialEditingId) {
      setEditingId(initialEditingId);
    }
  }, [initialEditingId]);

  useEffect(() => {
    if (!editingId && designSystems.length > 0) {
      setEditingId(selectedDesignSystemId ?? designSystems[0].id);
    }
  }, [designSystems, editingId, selectedDesignSystemId]);

  useEffect(() => {
    if (!editingDesignSystem) {
      setDraftName("");
      setDraftContent("");
      return;
    }

    setDraftName(editingDesignSystem.name);
    setDraftContent(editingDesignSystem.content);
  }, [editingDesignSystem]);

  const isDirty =
    editingDesignSystem !== null &&
    (draftName !== editingDesignSystem.name ||
      draftContent !== editingDesignSystem.content);

  const isDefault =
    editingDesignSystem !== null &&
    selectedDesignSystemId === editingDesignSystem.id;

  async function handleCreate() {
    try {
      setIsSaving(true);
      const created = await createDesignSystem({
        name: `Design system ${designSystems.length + 1}`,
        content: NEW_DESIGN_SYSTEM_CONTENT,
      });
      setEditingId(created.id);
      toast.success(t("designCreated"));
    } catch (error) {
      console.error("Failed to create design system", error);
      toast.error(t("designCreateFailed"));
    } finally {
      setIsSaving(false);
    }
  }

  async function handleSave() {
    if (!editingDesignSystem) {
      return;
    }

    const name = draftName.trim();
    if (!name) {
      toast.error(t("designSystemNameRequired"));
      return;
    }

    try {
      setIsSaving(true);
      await updateDesignSystem(editingDesignSystem.id, {
        name,
        content: draftContent,
      });
      toast.success(t("designSaved"));
    } catch (error) {
      console.error("Failed to save design system", error);
      toast.error(t("designSaveFailed"));
    } finally {
      setIsSaving(false);
    }
  }

  async function handleDelete() {
    if (!editingDesignSystem) {
      return;
    }

    if (
      !window.confirm(
        `Delete "${editingDesignSystem.name}"? This cannot be undone.`
      )
    ) {
      return;
    }

    try {
      setIsSaving(true);
      await deleteDesignSystem(editingDesignSystem.id);
      if (selectedDesignSystemId === editingDesignSystem.id) {
        setSelectedDesignSystemId(null);
      }
      const next = designSystems.find(
        (item) => item.id !== editingDesignSystem.id
      );
      setEditingId(next?.id ?? null);
      toast.success(t("designDeleted"));
    } catch (error) {
      console.error("Failed to delete design system", error);
      toast.error(t("designDeleteFailed"));
    } finally {
      setIsSaving(false);
    }
  }

  return (
    <div className="space-y-5">
      <div className="flex gap-2">
        <Select
          value={editingId ?? ""}
          onValueChange={setEditingId}
          disabled={designSystems.length === 0}
        >
          <SelectTrigger
            className="flex-1"
            data-testid="manage-design-system-select"
          >
            <SelectValue placeholder={t("noDesignSystems")} />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              {designSystems.map((designSystem) => (
                <SelectItem key={designSystem.id} value={designSystem.id}>
                  {designSystem.name}
                  {selectedDesignSystemId === designSystem.id && (
                    <span className="ml-2 text-xs text-violet-600 dark:text-violet-400">
                      {t("current")}
                    </span>
                  )}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
        <Button
          type="button"
          variant="outline"
          onClick={handleCreate}
          disabled={isSaving}
        >
          {t("newProject")}
        </Button>
      </div>

      {editingDesignSystem ? (
        <div className="space-y-4">
          <div>
            <label
              htmlFor="design-system-name"
              className="mb-1.5 block text-xs font-medium text-gray-700 dark:text-zinc-300"
            >
              {t("designSystemName")}
            </label>
            <div className="flex items-center gap-2">
              <Input
                id="design-system-name"
                value={draftName}
                onChange={(event) => setDraftName(event.target.value)}
                placeholder={t("designSystemNamePlaceholder")}
                data-testid="design-system-name"
              />
              {isDefault && (
                <span className="shrink-0 rounded-full bg-violet-100 px-2 py-0.5 text-xs font-medium text-violet-700 dark:bg-violet-900/40 dark:text-violet-300">
                  {t("current")}
                </span>
              )}
            </div>
          </div>

          <div>
            <label
              htmlFor="design-system-content"
              className="mb-1.5 block text-xs font-medium text-gray-700 dark:text-zinc-300"
            >
              {t("designSystemContent")}
            </label>
            <Textarea
              id="design-system-content"
              value={draftContent}
              onChange={(event) => setDraftContent(event.target.value)}
              placeholder={t("designSystemContentPlaceholder")}
              className="min-h-[240px] font-mono text-xs"
              data-testid="design-system-content"
            />
          </div>

          <div className="flex items-center justify-between border-t border-gray-100 pt-4 dark:border-zinc-800">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={handleDelete}
              disabled={isSaving}
              className="text-red-600 hover:bg-red-50 hover:text-red-700 dark:text-red-400 dark:hover:bg-red-950/40 dark:hover:text-red-300"
            >
              {t("commonDelete")}
            </Button>
            <div className="flex items-center gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() =>
                  setSelectedDesignSystemId(
                    isDefault ? null : editingDesignSystem.id
                  )
                }
                disabled={isSaving}
              >
                {isDefault ? t("removeDefault") : t("setDefault")}
              </Button>
              <Button
                type="button"
                size="sm"
                onClick={handleSave}
                disabled={isSaving || !isDirty}
              >
                {isDirty ? t("saveChanges") : t("saved")}
              </Button>
            </div>
          </div>
        </div>
      ) : (
        <div className="rounded-md border border-dashed border-gray-200 px-4 py-10 text-center text-sm text-gray-500 dark:border-zinc-700 dark:text-zinc-400">
          {t("noDesignSystems")}. <span className="font-medium">{t("newProject")}</span>
        </div>
      )}
    </div>
  );
}

export default DesignSystemsManager;
