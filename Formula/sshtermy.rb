# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.173"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.173/sshtermy_darwin_arm64"
      sha256 "922b257df0f0456d8a2af89daf1a20edade3326140b3c68d876faaa7fab257ca"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.173/sshtermy_darwin_amd64"
      sha256 "97fd8365b62a778df6720330ea3a26cdce4fff2a3a73e32a6b630fad9284e879"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.173/sshtermy_linux_arm64"
      sha256 "cac9b5d811e84c8a98eb63cd423de1bc878d89baec394935ecfc0ee992a6e786"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.173/sshtermy_linux_amd64"
      sha256 "a786b0e1aef4b74df0350da42aa1855692d128ee94f9553146ea322fba4918ba"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
