import { describe, expect, it } from "vitest";

import type { ManageGalleryItem } from "./types";
import { buildGalleryMediaFolderTree, flattenGalleryMediaFolders, galleryMediaFileName, groupGalleryMedia, moveGalleryMediaFolderTreeNode, naturalFileNameCompare } from "./galleryMediaFolders";

function item(uuid: string, relativePath: string, mediaKind = "STATIC_IMAGE", imageCategory = "PHOTO"): ManageGalleryItem {
  return {
    uuid, relativePath, mediaKind, imageCategory, contentFormat: mediaKind === "VIDEO" ? "MP4" : "JPEG",
    position: "1024", caption: "", excluded: false, availability: "AVAILABLE", processingState: "READY", byteSize: 1,
    videoProbeState: "", videoErrorCode: "", videoContainer: "", videoDurationSeconds: 0,
    videoWidth: 0, videoHeight: 0, videoCodec: "", audioCodec: "",
  };
}

describe("Gallery media folder grouping", () => {
  it("uses the complete parent path, keeps current file order and fixes root first inside each media group", () => {
    const groups = groupGalleryMedia([
      item("a-10", "disc-a/chapter-1/10.jpg"),
      item("b-1", "disc-b/1.jpg"),
      item("a-2", "disc-a/chapter-1/2.jpg"),
      item("root", "cover.jpg"),
      item("video", "disc-a/chapter-1/clip.mp4", "VIDEO", ""),
    ]);

    expect(groups.map((group) => group.key)).toEqual(["PHOTO", "VIDEO"]);
    expect(groups[0].folders.map((folder) => folder.path)).toEqual(["", "disc-a/chapter-1", "disc-b"]);
    expect(groups[0].folders[1].items.map((value) => value.uuid)).toEqual(["a-10", "a-2"]);
    expect(flattenGalleryMediaFolders(groups[0].folders).map((value) => value.uuid)).toEqual(["root", "a-10", "a-2", "b-1"]);
  });

  it("naturally sorts by filename instead of the complete parent path", () => {
    const values = [item("10", "a/10.jpg"), item("2", "a/2.jpg"), item("1", "a/1.jpg")];
    values.sort(naturalFileNameCompare);
    expect(values.map((value) => galleryMediaFileName(value.relativePath))).toEqual(["1.jpg", "2.jpg", "10.jpg"]);
  });

  it("builds nested directory nodes, including directories without direct media", () => {
    const folders = groupGalleryMedia([
      item("deep", "disc-a/chapter-1/01.jpg"), item("other", "disc-b/02.jpg"),
      item("direct", "disc-a/cover.jpg"), item("root", "cover.jpg"),
    ])[0].folders;
    const tree = buildGalleryMediaFolderTree(folders);
    expect(tree.totalCount).toBe(4);
    expect(tree.items.map((value) => value.uuid)).toEqual(["root"]);
    expect(tree.children.map((child) => [child.path, child.totalCount])).toEqual([["disc-a", 2], ["disc-b", 1]]);
    expect(tree.children[0].children[0].path).toBe("disc-a/chapter-1");
    expect(tree.children[0].items.map((value) => value.uuid)).toEqual(["direct"]);
  });

  it("moves only sibling subtrees and keeps root/direct media before their descendants", () => {
    const folders = groupGalleryMedia([
      item("deep", "disc-a/chapter-1/01.jpg"), item("other", "disc-b/02.jpg"),
      item("direct", "disc-a/cover.jpg"), item("root", "cover.jpg"),
    ])[0].folders;
    expect(moveGalleryMediaFolderTreeNode(folders, "disc-b", -1)?.map((folder) => folder.path)).toEqual(["", "disc-b", "disc-a", "disc-a/chapter-1"]);
    expect(moveGalleryMediaFolderTreeNode(folders, "disc-a/chapter-1", -1)).toBeNull();
    expect(moveGalleryMediaFolderTreeNode(folders, "", 1)).toBeNull();
  });
});
