package com.github.kr328.clash.service.util

import java.io.File
import java.io.IOException

/** Keep the previous validated download until the next replacement succeeds. */
internal fun replaceProfileDirectory(source: File, target: File) {
    val stage = File(target.parentFile, target.name + ".candidate")
    val previous = File(target.parentFile, target.name + ".previous")
    if (!target.exists() && previous.exists() && !previous.renameTo(target))
        throw IOException("Cannot recover previous profile")
    stage.deleteRecursively()
    check(source.copyRecursively(stage, overwrite = true)) { "Cannot stage profile" }
    if (previous.exists() && !previous.deleteRecursively()) throw IOException("Cannot replace previous profile backup")
    val hadPrevious = target.exists()
    if (hadPrevious && !target.renameTo(previous)) throw IOException("Cannot preserve previous profile")
    if (!stage.renameTo(target)) {
        if (hadPrevious && !previous.renameTo(target)) throw IOException("Profile activation failed; previous profile remains at ${previous.name}")
        throw IOException("Profile activation failed; previous profile restored")
    }
}
