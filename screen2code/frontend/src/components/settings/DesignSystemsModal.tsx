import { DesignSystem } from "../../types";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "../ui/dialog";
import DesignSystemsManager from "./DesignSystemsManager";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  designSystems: DesignSystem[];
  selectedDesignSystemId: string | null;
  setSelectedDesignSystemId: (id: string | null) => void;
  initialEditingId: string | null;
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

function DesignSystemsModal({
  open,
  onOpenChange,
  designSystems,
  selectedDesignSystemId,
  setSelectedDesignSystemId,
  initialEditingId,
  createDesignSystem,
  updateDesignSystem,
  deleteDesignSystem,
}: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>设计规范</DialogTitle>
          <DialogDescription>

            设置每次生成使用的配色、字体与布局规则。
          </DialogDescription>
        </DialogHeader>
        <DesignSystemsManager
          designSystems={designSystems}
          selectedDesignSystemId={selectedDesignSystemId}
          setSelectedDesignSystemId={setSelectedDesignSystemId}
          initialEditingId={initialEditingId}
          createDesignSystem={createDesignSystem}
          updateDesignSystem={updateDesignSystem}
          deleteDesignSystem={deleteDesignSystem}
        />
      </DialogContent>
    </Dialog>
  );
}

export default DesignSystemsModal;
