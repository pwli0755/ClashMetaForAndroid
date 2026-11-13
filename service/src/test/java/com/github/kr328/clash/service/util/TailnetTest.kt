package com.github.kr328.clash.service.util

import com.github.kr328.clash.service.model.AccessControlMode
import org.junit.Assert.*
import org.junit.Test

class TailnetTest {
    private val packages = mapOf(10001 to listOf("one"), 10002 to listOf("two", "shared"), 10003 to emptyList(), 999 to listOf("system"))
    @Test fun existingModesAreNotReinterpreted() {
        val selected = setOf("one", "two")
        repeat(2) {
            assertEquals(listOf(10001), proxyUIDs(AccessControlMode.AcceptSelected, selected, packages))
            assertEquals(emptyList<Int>(), proxyUIDs(AccessControlMode.DenySelected, selected, packages))
            assertEquals(listOf(10001,10002), proxyUIDs(AccessControlMode.AcceptAll, selected, packages))
            assertEquals(setOf("one", "two"), selected)
        }
    }
    @Test fun reinstallAndSharedUidFailClosed() {
        assertEquals(emptyList<Int>(), proxyUIDs(AccessControlMode.AcceptSelected, setOf("one"), mapOf(10001 to listOf("replacement"))))
        assertEquals(listOf(10011), proxyUIDs(AccessControlMode.AcceptSelected, setOf("one"), mapOf(10011 to listOf("one"))))
        assertEquals(listOf(10002), proxyUIDs(AccessControlMode.AcceptSelected, setOf("two", "shared"), packages))
        assertEquals(emptyList<Int>(), proxyUIDs(AccessControlMode.AcceptAll, emptySet(), emptyMap()))
    }
}
