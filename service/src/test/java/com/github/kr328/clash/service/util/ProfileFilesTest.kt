package com.github.kr328.clash.service.util

import java.nio.file.Files
import org.junit.Assert.*
import org.junit.Test

class ProfileFilesTest {
    @Test fun failedCandidatePreservesPreviousDownload() {
        val root = Files.createTempDirectory("cmfa-profile").toFile()
        try {
            val target = root.resolve("profile").apply { mkdir() }
            target.resolve("config.yaml").writeText("A")
            val source = root.resolve("download").apply { mkdir() }
            source.resolve("config.yaml").writeText("B")
            replaceProfileDirectory(source, target)
            assertEquals("B", target.resolve("config.yaml").readText())
            assertEquals("A", root.resolve("profile.previous/config.yaml").readText())
            try {
                replaceProfileDirectory(root.resolve("missing"), target)
                fail("missing candidate accepted")
            } catch (_: java.io.IOException) { }
            assertEquals("B", target.resolve("config.yaml").readText())
            assertEquals("A", root.resolve("profile.previous/config.yaml").readText())
        } finally { root.deleteRecursively() }
    }
    @Test fun interruptedActivationRecoversBeforeNextAttempt() {
        val root = Files.createTempDirectory("cmfa-profile").toFile()
        try {
            root.resolve("profile.previous").mkdir()
            root.resolve("profile.previous/config.yaml").writeText("A")
            try { replaceProfileDirectory(root.resolve("missing"), root.resolve("profile")) }
            catch (_: java.io.IOException) { }
            assertEquals("A", root.resolve("profile/config.yaml").readText())
        } finally { root.deleteRecursively() }
    }
}
